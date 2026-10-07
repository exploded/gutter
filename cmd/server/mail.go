package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"

	"gutter/db"
	"gutter/mailer"
)

// ── Email notifications (Amazon SES, or Gmail once connected) ──
//
// SES is configured by AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_REGION;
// the sender and notification address are CONTACT_EMAIL. When the admin
// connects Gmail (gmail.go), customer mail goes from that account instead.
// With neither configured every notify* call is a silent no-op, so local dev
// works without any AWS or Google setup.

var (
	mail        *mailer.Mailer
	notifyEmail string // where booking notifications go
)

func initMail() {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "ap-southeast-2"
	}
	// Sender and notification target both derive from CONTACT_EMAIL; the
	// address must be a verified SES identity.
	from := site.Name + " <" + site.Email + ">"
	notifyEmail = site.Email
	mail = mailer.New(region, os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), from)
	if mail.Enabled() {
		log.Printf("email: SES enabled, from=%q notify=%q region=%s", from, notifyEmail, region)
	} else {
		log.Printf("email: not configured (set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY) — notifications disabled")
	}
}

// mailEnabled reports whether any route can send.
func mailEnabled() bool {
	return mail.Enabled() || gmail.Load() != nil
}

// send delivers an email in the background and logs failures. Callers must
// not depend on delivery; the request has already been persisted.
func send(to, subject, html, text, replyTo string) {
	if !mailEnabled() || to == "" {
		return
	}
	go func() {
		if err := deliver(to, subject, html, text, replyTo); err != nil {
			log.Printf("email: %v", err)
		}
	}()
}

// sendNow delivers synchronously (with optional attachments) and returns the
// error, for admin actions where the outcome must be shown and a state change
// should only happen on success. When email is not configured it logs and
// returns nil so local flows still complete.
func sendNow(to, subject, html, text, replyTo string, atts ...mailer.Attachment) error {
	if to == "" {
		return fmt.Errorf("no recipient address")
	}
	if !mailEnabled() {
		log.Printf("email: (disabled) would send %q to %s with %d attachment(s)", subject, to, len(atts))
		return nil
	}
	return deliver(to, subject, html, text, replyTo, atts...)
}

// deliver picks the route for one message. Customer mail goes through Gmail
// when it's connected, falling back to SES if Gmail fails. Admin notices stay
// on SES: CONTACT_EMAIL forwards to the same Gmail account, and Gmail drops a
// forwarded copy of a message it has already filed in Sent, so a booking alert
// sent from that account would never reach the inbox.
func deliver(to, subject, html, text, replyTo string, atts ...mailer.Attachment) error {
	g := gmail.Load()
	if g != nil && (!mail.Enabled() || !strings.EqualFold(to, notifyEmail)) {
		// Replies should land straight in the Gmail thread, not take the long
		// way round through the CONTACT_EMAIL forward.
		if strings.EqualFold(replyTo, site.Email) {
			replyTo = ""
		}
		err := g.send(to, subject, html, text, replyTo, atts...)
		if err == nil || !mail.Enabled() {
			return err
		}
		log.Printf("email: Gmail failed, sending %q to %s through SES instead", subject, to)
	}
	return mail.SendWithAttachments(to, subject, html, text, replyTo, atts...)
}

const mailTmplSrc = `
{{define "wrap"}}<!doctype html><html><body style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.5;color:#1c1c1c;margin:0;padding:24px;background:#f6f5f2">
<div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #e6e3dc;border-radius:8px;padding:28px">
{{.}}
<p style="margin-top:32px;font-size:12px;color:#777">{{site.Name}} — {{site.Owner}}, Melbourne's north-east · <a href="{{site.BaseURL}}" style="color:#777">{{site.BaseURL}}</a></p>
</div></body></html>{{end}}

{{define "row"}}<tr><td style="padding:6px 12px 6px 0;color:#666;white-space:nowrap;vertical-align:top">{{.K}}</td><td style="padding:6px 0;vertical-align:top">{{.V}}</td></tr>{{end}}

{{define "booking-admin"}}
<h2 style="margin:0 0 4px">New booking request #{{.ID}}</h2>
{{if .Suspicious}}<p style="background:#fff3cd;border:1px solid #ffe69c;padding:8px 12px;border-radius:6px"><strong>Flagged as suspicious</strong> — form timing looked automated. Marked as spam in admin; treat with care.</p>{{end}}
<table style="border-collapse:collapse;margin:12px 0 20px">
{{template "row" (kv "Name" .B.Name)}}
{{template "row" (kv "Phone" .B.Phone)}}
{{template "row" (kv "Email" .B.Email)}}
{{template "row" (kv "Address" .B.Address)}}
{{template "row" (kv "Service" .ServiceTitle)}}
{{template "row" (kv "Property" .Property)}}
{{template "row" (kv "Price shown" .Price)}}
{{template "row" (kv "Preferred time" .B.PreferredTime)}}
{{template "row" (kv "Found you via" (srcLabel .B.Source))}}
{{template "row" (kv "IP" .B.IP)}}
</table>
{{if .B.Issue}}<p style="margin:0 0 6px;color:#666">Their notes:</p>
<blockquote style="margin:0 0 20px;padding:12px 16px;border-left:3px solid #d9d5cc;background:#faf9f6;white-space:pre-wrap">{{.B.Issue}}</blockquote>{{end}}
<p><a href="{{site.BaseURL}}/admin" style="display:inline-block;background:#1c1c1c;color:#fff;text-decoration:none;padding:10px 16px;border-radius:6px">Open admin</a></p>
{{end}}

{{define "booking-customer"}}
<h2 style="margin:0 0 12px">Thanks {{.B.Name}} — I've got your request.</h2>
<p>I'll be in touch within a day to lock in a time. If it's urgent — say a council Fire Prevention Notice deadline — just reply to this email{{if site.Phone}} or call {{site.Phone}}{{end}}.</p>
<p style="margin:20px 0 6px;color:#666">What you booked:</p>
<table style="border-collapse:collapse;margin:0 0 20px">
{{if .ServiceTitle}}{{template "row" (kv "Service" .ServiceTitle)}}{{end}}
{{if .Property}}{{template "row" (kv "Property" .Property)}}{{end}}
{{if .Price}}{{template "row" (kv "Price" .Price)}}{{end}}
{{if .B.Address}}{{template "row" (kv "Address" .B.Address)}}{{else if .B.Suburb}}{{template "row" (kv "Suburb" .B.Suburb)}}{{end}}
{{if .B.PreferredTime}}{{template "row" (kv "Preferred time" .B.PreferredTime)}}{{end}}
</table>
{{if .B.Issue}}<blockquote style="margin:0 0 20px;padding:12px 16px;border-left:3px solid #d9d5cc;background:#faf9f6;white-space:pre-wrap">{{.B.Issue}}</blockquote>{{end}}
<p style="color:#666">You don't need to be home on the day, as long as I can get to the side of the house. Please let me know about locked gates, dogs, or solar panels on the roof.</p>
<p>— {{site.Owner}}</p>
{{end}}

{{define "gmail-test"}}
<h2 style="margin:0 0 12px">Gmail is connected</h2>
<p>This test came from the {{site.Name}} admin. Confirmations, reminders, invoices and receipts now go to customers from <strong>{{.From}}</strong>, and they'll sit in that account's Sent mail.</p>
{{end}}
`

var mailTmpl = template.Must(template.Must(template.New("mail").Funcs(template.FuncMap{
	"site":     func() *siteConfig { return &site },
	"kv":       func(k, v string) map[string]string { return map[string]string{"K": k, "V": v} },
	"money":    fmtCents,
	"btn":      func(href, label string) map[string]string { return map[string]string{"Href": href, "Label": label} },
	"srcLabel": db.SourceLabel,
}).Parse(mailTmplSrc)).Parse(mailBillingTmplSrc + mailSchedulerTmplSrc))

// renderMail executes a named body template inside the "wrap" chrome.
func renderMail(name string, data any) (string, error) {
	var body bytes.Buffer
	if err := mailTmpl.ExecuteTemplate(&body, name, data); err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := mailTmpl.ExecuteTemplate(&out, "wrap", template.HTML(body.String())); err != nil {
		return "", err
	}
	return out.String(), nil
}

// notifyBooking emails the admin (always) and the customer (when they gave an
// address and the submission wasn't flagged) after a booking is stored.
func notifyBooking(id int64, b *db.Booking, suspicious bool) {
	if !mailEnabled() {
		return
	}
	svcTitle := ""
	if s, ok := findService(b.ServiceSlug); ok {
		svcTitle = s.Title
	}
	property, price := bookingSummary(b)
	data := map[string]any{"ID": id, "B": b, "ServiceTitle": svcTitle, "Property": property, "Price": price, "Suspicious": suspicious}

	// Admin notice — reply goes straight to the customer.
	subj := fmt.Sprintf("Booking #%d: %s", id, b.Name)
	if svcTitle != "" {
		subj += " — " + svcTitle
	}
	if suspicious {
		subj = "[suspicious] " + subj
	}
	html, err := renderMail("booking-admin", data)
	if err != nil {
		log.Printf("email: render booking-admin: %v", err)
		return
	}
	text := fmt.Sprintf("New booking request #%d\n\nName: %s\nPhone: %s\nEmail: %s\nAddress: %s\nService: %s\nProperty: %s\nPrice shown: %s\nPreferred time: %s\n\n%s\n\n%s/admin\n",
		id, b.Name, b.Phone, b.Email, b.Address, svcTitle, property, price, b.PreferredTime, b.Issue, site.BaseURL)
	send(notifyEmail, subj, html, text, b.Email)

	// Customer confirmation
	if b.Email == "" || suspicious {
		return
	}
	html, err = renderMail("booking-customer", data)
	if err != nil {
		log.Printf("email: render booking-customer: %v", err)
		return
	}
	text = fmt.Sprintf("Thanks %s — I've got your request and will be in touch within a day to lock in a time.\n\nProperty: %s\nPrice: %s\nAddress: %s\n\n— %s\n%s\n",
		b.Name, property, price, b.Address, site.Owner, site.BaseURL)
	send(b.Email, "Got your booking request — "+site.Name, html, text, site.Email)
}

// bookingSummary describes what was booked, for emails: the property row with
// any extras, and the price the customer was shown.
func bookingSummary(b *db.Booking) (property, price string) {
	if p, ok := findPropertyType(b.PropertyType); ok {
		property = p.Name
	}
	if b.OnPlan {
		property += ", Fire-ready plan"
	}
	if b.QuoteCents > 0 {
		price = fmtCents(b.QuoteCents)
	}
	return property, price
}
