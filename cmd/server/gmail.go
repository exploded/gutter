package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	netmail "net/mail"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"gutter/db"
	"gutter/mailer"
)

// Gmail sending. Once the primary admin connects their Gmail account from
// /admin/email, customer emails (confirmations, reminders, invoices, receipts)
// go out from that account through the Gmail API, so they sit in Vin's Sent
// mail and replies thread with them. Admin notices stay on SES (see deliver in
// mail.go). Disconnecting puts everything back on SES.
//
// Like calendar sync, connecting is separate from admin sign-in and has its own
// redirect URI, so the send scope is only ever granted from the settings page.

// gmailScope lets the app send mail as the user. It can't read, list or
// delete anything in the mailbox.
const gmailScope = "https://www.googleapis.com/auth/gmail.send"

const gmailSendURL = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"

// gmailOAuth is the OAuth config for the send scope. nil when GOOGLE_CLIENT_ID
// is unset.
var gmailOAuth *oauth2.Config

func initGmailOAuth() {
	cid := os.Getenv("GOOGLE_CLIENT_ID")
	if cid == "" {
		return
	}
	gmailOAuth = &oauth2.Config{
		ClientID:     cid,
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  site.BaseURL + "/auth/google/mail/callback",
		// email lets the callback check which account was connected.
		Scopes:   []string{gmailScope, "email"},
		Endpoint: google.Endpoint,
	}
}

// gmailSender sends through one connected account. The oauth2 client caches
// the access token, so it is built once per connection, not per message.
type gmailSender struct {
	email    string
	hc       *http.Client
	endpoint string // gmailSendURL; tests point it at a local server
}

// gmail is the live connection, nil when Gmail isn't connected.
var gmail atomic.Pointer[gmailSender]

// gmailFrom is the connected Gmail address, or "" when mail goes through SES.
func gmailFrom() string {
	if s := gmail.Load(); s != nil {
		return s.email
	}
	return ""
}

// loadGmail reads the stored connection at startup.
func loadGmail() {
	g, err := db.GetGoogleMail()
	if err != nil {
		log.Printf("gmail: load connection: %v", err)
		return
	}
	setGmail(g)
	if s := gmail.Load(); s != nil {
		log.Printf("email: customer mail goes from %s via Gmail", s.email)
	}
}

// setGmail swaps the live sender for the given connection (nil disconnects).
func setGmail(g *db.GoogleMail) {
	if gmailOAuth == nil || !g.Connected() {
		gmail.Store(nil)
		return
	}
	src := gmailOAuth.TokenSource(context.Background(), &oauth2.Token{RefreshToken: g.RefreshToken})
	gmail.Store(&gmailSender{
		email:    g.AccountEmail,
		hc:       oauth2.NewClient(context.Background(), src),
		endpoint: gmailSendURL,
	})
}

// send delivers one message and records the outcome for the settings page.
func (s *gmailSender) send(to, subject, html, text, replyTo string, atts ...mailer.Attachment) error {
	err := s.post(to, subject, html, text, replyTo, atts...)
	if err != nil {
		log.Printf("gmail: %v", err)
		if e := db.SetGoogleMailError(time.Now().In(db.Melbourne).Format("2 Jan 3:04 pm") + ": " + err.Error()); e != nil {
			log.Printf("gmail: record error: %v", e)
		}
		return err
	}
	if e := db.MarkGoogleMailSent(); e != nil {
		log.Printf("gmail: record send: %v", e)
	}
	return nil
}

func (s *gmailSender) post(to, subject, html, text, replyTo string, atts ...mailer.Attachment) error {
	from := (&netmail.Address{Name: site.Name, Address: s.email}).String()
	raw, err := mailer.BuildMessage(from, to, replyTo, subject, html, text, atts...)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}
	body, err := json.Marshal(map[string]string{"raw": base64.URLEncoding.EncodeToString(raw)})
	if err != nil {
		return err
	}
	// Bounded like the SES call, so a stuck request can't hang a handler.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("gmail send to %s: %w", to, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("gmail send to %s: %s: %s", to, resp.Status, googleErrMessage(msg))
	}
	return nil
}

// ── Connect / disconnect ──

func handleGmailConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if gmailOAuth == nil {
		redirectMsg(w, r, "/admin/email", "err", "Google OAuth is not configured on this server.")
		return
	}
	state := generateSessionToken()
	oauthStates.Store(state, time.Now().Add(10*time.Minute))
	// access_type=offline + prompt=consent guarantees a refresh token even when
	// the account has authorised this client before.
	authURL := gmailOAuth.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("login_hint", adminEmail()))
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func handleGmailCallback(w http.ResponseWriter, r *http.Request) {
	const back = "/admin/email"
	w.Header().Set("Cache-Control", "no-store")
	if gmailOAuth == nil {
		http.Error(w, "Google OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	val, ok := oauthStates.LoadAndDelete(r.URL.Query().Get("state"))
	if !ok || time.Now().After(val.(time.Time)) {
		redirectMsg(w, r, back, "err", "That connection attempt expired. Press Connect again.")
		return
	}
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		redirectMsg(w, r, back, "err", "Google declined the request: "+errParam)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gcalTimeout)
	defer cancel()

	tok, err := gmailOAuth.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("gmail: token exchange: %v", err)
		redirectMsg(w, r, back, "err", "Could not complete the Google connection.")
		return
	}
	if tok.RefreshToken == "" {
		redirectMsg(w, r, back, "err", "Google did not return a refresh token. Remove this app at myaccount.google.com/permissions and connect again.")
		return
	}
	// Google lets the user untick the send permission on the consent screen.
	if !strings.Contains(fmt.Sprint(tok.Extra("scope")), gmailScope) {
		redirectMsg(w, r, back, "err", "Google didn't grant permission to send email. Connect again and leave the \"Send email on your behalf\" box ticked.")
		return
	}

	// Mail must go out from the business's own account, not a support login.
	email, err := googleAccountEmail(ctx, gmailOAuth.Client(ctx, tok))
	if err != nil {
		log.Printf("gmail: userinfo: %v", err)
		redirectMsg(w, r, back, "err", "Could not read the Google account details.")
		return
	}
	if !strings.EqualFold(email, adminEmail()) {
		redirectMsg(w, r, back, "err", "That is "+email+", not the admin account ("+adminEmail()+"). Sign out of Google and try again.")
		return
	}

	if err := db.SaveGoogleMail(email, tok.RefreshToken); err != nil {
		log.Printf("gmail: save connection: %v", err)
		redirectMsg(w, r, back, "err", "Could not save the connection.")
		return
	}
	setGmail(&db.GoogleMail{AccountEmail: email, RefreshToken: tok.RefreshToken})
	redirectMsg(w, r, back, "ok", "Connected. Customer emails now go from "+email+". Send yourself a test to check.")
}

func handleGmailDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := db.DisconnectGoogleMail(); err != nil {
		log.Printf("gmail: disconnect: %v", err)
		redirectMsg(w, r, "/admin/email", "err", "Could not disconnect.")
		return
	}
	setGmail(nil)
	redirectMsg(w, r, "/admin/email", "ok", "Disconnected. Email goes through Amazon SES again.")
}
