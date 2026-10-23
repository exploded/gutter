package main

import (
	"fmt"
	"net/http"

	"gutter/db"
)

// /admin/email — connect or disconnect Gmail, and send a test message.

type emailSettingsData struct {
	Flash       flash
	Configured  bool // GOOGLE_CLIENT_ID present, so connecting is possible
	Conn        *db.GoogleMail
	AdminEmail  string
	NotifyEmail string
}

func handleAdminEmailSettings(w http.ResponseWriter, r *http.Request) {
	conn, err := db.GetGoogleMail()
	if err != nil {
		logErrorf("gmail settings: %v", err)
		http.Error(w, "failed to load email settings", http.StatusInternalServerError)
		return
	}
	render(w, r, "admin-email", emailSettingsData{
		Flash:       readFlash(r),
		Configured:  gmailOAuth != nil,
		Conn:        conn,
		AdminEmail:  adminEmail(),
		NotifyEmail: notifyEmail,
	})
}

// handleAdminEmailTest sends a test message through Gmail to whoever is
// signed in, so a failure shows Google's reason.
func handleAdminEmailTest(w http.ResponseWriter, r *http.Request) {
	g := gmail.Load()
	if g == nil {
		emailSettingsMsg(w, r, http.StatusUnprocessableEntity, "err", "Gmail is not connected.")
		return
	}
	to := ""
	if sess := getSession(r); sess != nil && sess.User != nil {
		to = sess.User.Email
	}
	if to == "" {
		emailSettingsMsg(w, r, http.StatusUnprocessableEntity, "err", "Couldn't tell who you're signed in as.")
		return
	}
	html, err := renderMail("gmail-test", map[string]string{"From": g.email})
	if err != nil {
		logErrorf("gmail test: render: %v", err)
		emailSettingsMsg(w, r, http.StatusInternalServerError, "err", "Could not build the test email.")
		return
	}
	text := fmt.Sprintf("This is a test from the %s admin. Customer emails now go from %s.\n", site.Name, g.email)
	if err := g.send(to, "Test email from "+site.Name, html, text, ""); err != nil {
		emailSettingsMsg(w, r, http.StatusBadGateway, "err", "Gmail refused it: "+err.Error())
		return
	}
	emailSettingsMsg(w, r, http.StatusOK, "ok", "Sent a test to "+to+". It should also be in Sent mail on "+g.email+".")
}

// emailSettingsMsg finishes a settings action: the htmx reply is just the flash
// region; a plain form post redirects.
func emailSettingsMsg(w http.ResponseWriter, r *http.Request, code int, key, msg string) {
	if !isHTMX(r) {
		redirectMsg(w, r, "/admin/email", key, msg)
		return
	}
	d := emailSettingsData{}
	if key == "ok" {
		d.Flash = flash{OK: msg}
	} else {
		d.Flash = flash{Err: msg}
	}
	renderFragment(w, r, "admin-email", "flash-response", code, d)
}
