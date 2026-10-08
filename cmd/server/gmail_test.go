package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gutter/db"
	"gutter/mailer"
)

// fakeGmail stands in for the Gmail send endpoint and keeps each raw message.
type fakeGmail struct {
	mu     sync.Mutex
	status int // reply status; 0 = 200
	msgs   []*netmail.Message
}

func (f *fakeGmail) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Raw string `json:"raw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw, err := base64.URLEncoding.DecodeString(body.Raw)
	if err != nil {
		http.Error(w, "raw is not base64url", http.StatusBadRequest)
		return
	}
	m, err := netmail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		http.Error(w, "unparseable message", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		io.WriteString(w, `{"error":{"message":"Invalid Credentials"}}`)
		return
	}
	f.msgs = append(f.msgs, m)
	io.WriteString(w, `{"id":"m1"}`)
}

func TestGmailDelivery(t *testing.T) {
	if err := db.Open(filepath.Join(t.TempDir(), "gmail.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	site = siteConfig{Name: businessName, BaseURL: "https://example.test", Email: "vin@example.test"}
	mail, notifyEmail = nil, "vin@example.test"

	// Not connected and no SES: sends are logged no-ops.
	gmail.Store(nil)
	if err := sendNow("ann@example.test", "Hi", "<p>hi</p>", "hi", site.Email); err != nil {
		t.Fatalf("disabled send: %v", err)
	}

	fake := &fakeGmail{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	if err := db.SaveGoogleMail("vin.spout@gmail.com", "refresh"); err != nil {
		t.Fatal(err)
	}
	gmail.Store(&gmailSender{email: "vin.spout@gmail.com", hc: srv.Client(), endpoint: srv.URL})
	defer gmail.Store(nil)

	att := mailer.Attachment{Filename: "INV-1001.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 \x00\xff binary")}
	if err := sendNow("ann@example.test", "Invoice INV-1001 — $279.00", "<p>Thanks</p>", "Thanks", site.Email, att); err != nil {
		t.Fatalf("gmail send: %v", err)
	}
	if len(fake.msgs) != 1 {
		t.Fatalf("gmail got %d messages, want 1", len(fake.msgs))
	}
	m := fake.msgs[0]
	from, err := netmail.ParseAddress(m.Header.Get("From"))
	if err != nil || from.Address != "vin.spout@gmail.com" || from.Name != businessName {
		t.Errorf("From = %q (%v), want the business name at the Gmail address", m.Header.Get("From"), err)
	}
	if got := m.Header.Get("Reply-To"); got != "" {
		t.Errorf("Reply-To = %q, want none so replies thread in Gmail", got)
	}
	if subj, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); subj != "Invoice INV-1001 — $279.00" {
		t.Errorf("Subject = %q", subj)
	}
	if g, _ := db.GetGoogleMail(); g.LastSentAt.IsZero() || g.LastError != "" {
		t.Errorf("connection after a send = %+v, want LastSentAt set and no error", g)
	}

	// The admin notice has no SES to use, so Gmail carries it too.
	if err := sendNow(notifyEmail, "Booking #1", "<p>x</p>", "x", "ann@example.test"); err != nil {
		t.Fatal(err)
	}
	if len(fake.msgs) != 2 || fake.msgs[1].Header.Get("Reply-To") != "ann@example.test" {
		t.Errorf("admin notice not sent through Gmail with the customer as Reply-To")
	}

	// A refusal comes back as the error (no SES to fall back to) and shows on
	// the settings page.
	fake.status = http.StatusUnauthorized
	err = sendNow("ann@example.test", "Hi", "<p>hi</p>", "hi", "")
	if err == nil || !strings.Contains(err.Error(), "Invalid Credentials") {
		t.Fatalf("refused send err = %v, want Google's message", err)
	}
	if g, _ := db.GetGoogleMail(); !strings.Contains(g.LastError, "Invalid Credentials") {
		t.Errorf("LastError = %q, want the refusal recorded", g.LastError)
	}
}
