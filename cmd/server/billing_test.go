package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gutter/db"
)

// sampleInvoiceView builds a representative invoice for template/PDF tests.
func sampleInvoiceView(status string) *invoiceView {
	start := time.Date(2026, 8, 20, 9, 30, 0, 0, db.Melbourne)
	inv := &db.Invoice{
		ID: 1, Number: 1001, BookingID: 5, CustomerID: 9, Status: status,
		IssuedAt: db.Today(), DueAt: db.Today().AddDate(0, 0, 7), TotalCents: 27900,
		PaymentLink: "https://pay.example/abc", Notes: "Thanks — the back downpipe is rusting through; worth a plumber.", ViewToken: strings.Repeat("a", 64),
	}
	if status == db.InvoicePaid {
		inv.PaidAt, inv.PaymentMethod, inv.PaymentRef = db.Today(), db.PayZellerLink, "Z-42"
	}
	return &invoiceView{
		Inv: inv,
		Items: []db.InvoiceItem{
			{Description: "Gutter clean — Unit or townhouse", Qty: 1, UnitCents: 21900},
			{Description: "Downpipe jetted — très bouché", Qty: 1, UnitCents: 9000},
			{Description: "Neighbour deal", Qty: 1, UnitCents: -3000},
		},
		Cust:         &db.Customer{ID: 9, Name: "Zoë O'Brien <b>x</b>", Email: "zoe@example.test", Phone: "0400 000 000", Address: "1 Test St", Suburb: "Donvale"},
		Booking:      &db.Booking{ID: 5, Name: "Zoë", Suburb: "Donvale", ServiceSlug: "gutter-cleaning", PropertyType: "unit", StartAt: start, DurationMin: 60, Status: db.BookingDone},
		ServiceTitle: "Gutter cleaning",
		PublicURL:    "https://example.test/invoice/" + strings.Repeat("a", 64),
	}
}

func TestBillingMailTemplates(t *testing.T) {
	site = siteConfig{Name: businessName, Brand: brandName, BaseURL: "https://example.test", Email: "me@example.test", Phone: "0400 000 000",
		ABN: "12 345 678 901", BankName: "Up The Spout", BankBSB: "000-000", BankAcct: "12345678"}
	b := &db.Booking{ID: 3, Name: "Ann <b>Bold</b>", Email: "ann@example.test", Suburb: "Donvale", ServiceSlug: "fire-ready-plan",
		PropertyType: "single", OnPlan: true, Issue: "Spring plan clean", StartAt: time.Date(2026, 8, 20, 9, 30, 0, 0, db.Melbourne),
		DurationMin: 45, ParentBookingID: 1}
	for _, tc := range []struct {
		name, want string
		data       any
	}{
		{"booking-confirm", "Your gutter clean has been moved", func() bookingMailData { d := newBookingMailData(b); d.Rescheduled = true; return d }()},
		{"booking-cancel", "Your gutter clean has been cancelled", func() bookingMailData { d := newBookingMailData(b); d.Reason = "rain"; return d }()},
	} {
		out, err := renderMail(tc.name, tc.data)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if strings.Contains(out, "<b>Bold</b>") || !strings.Contains(out, "Thursday 20 August 2026, 9:30 am") ||
			!strings.Contains(out, tc.want) || !strings.Contains(out, "<p>— Up The Spout</p>") {
			t.Errorf("%s: unexpected output:\n%s", tc.name, out)
		}
	}
	// A follow-up (the next plan clean) says so, rather than quoting the notes
	// back as a problem.
	if out, _ := renderMail("booking-confirm", newBookingMailData(b)); !strings.Contains(out, "This is a follow-up clean: Spring plan clean.") ||
		!strings.Contains(out, "Fire-ready plan") || !strings.Contains(out, "Your place in Donvale") {
		t.Errorf("booking-confirm (follow-up):\n%s", out)
	}
	for _, status := range []string{db.InvoiceSent, db.InvoicePaid} {
		v := sampleInvoiceView(status)
		name := map[string]string{db.InvoiceSent: "invoice-send", db.InvoicePaid: "invoice-receipt"}[status]
		out, err := renderMail(name, newInvoiceMailData(v))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(out, "<b>x</b>") || !strings.Contains(out, "INV-1001") || !strings.Contains(out, "$279.00") || !strings.Contains(out, v.PublicURL) {
			t.Errorf("%s: unexpected output:\n%s", name, out)
		}
		if status == db.InvoiceSent && (!strings.Contains(out, "https://pay.example/abc") || !strings.Contains(out, "000-000")) {
			t.Errorf("invoice-send: missing payment details:\n%s", out)
		}
	}
	// The review ask is opt-in: present only when the admin ticked the box and
	// REVIEW_URL is configured.
	site.ReviewURL = "https://g.page/r/TEST/review"
	v := sampleInvoiceView(db.InvoicePaid)
	d := newInvoiceMailData(v)
	off, err := renderMail("invoice-receipt", d)
	if err != nil {
		t.Fatalf("invoice-receipt: %v", err)
	}
	if strings.Contains(off, site.ReviewURL) || strings.Contains(off, "Google review") {
		t.Errorf("invoice-receipt: review ask leaked in without the tick-box:\n%s", off)
	}
	d.ReviewURL = site.ReviewURL
	on, err := renderMail("invoice-receipt", d)
	if err != nil {
		t.Fatalf("invoice-receipt with review: %v", err)
	}
	if !strings.Contains(on, site.ReviewURL) || !strings.Contains(on, "Leave a Google review") {
		t.Errorf("invoice-receipt: missing the review ask:\n%s", on)
	}

	// ICS is well-formed and UTC, and carries the business's own name and domain.
	ics := bookingICS(b, "Fire-ready plan")
	for _, want := range []string{"BEGIN:VEVENT", "DTSTART:20260819T233000Z", "DTEND:20260820T001500Z",
		"UID:booking-3@upthespout.com.au", "SUMMARY:Up The Spout — Fire-ready plan"} {
		if !strings.Contains(ics, want) {
			t.Errorf("ics missing %q:\n%s", want, ics)
		}
	}
	if ics := bookingICS(b, ""); !strings.Contains(ics, "SUMMARY:Up The Spout — gutter clean") {
		t.Errorf("ics without a service:\n%s", ics)
	}
	// nil mailer: sends are no-ops that succeed.
	mail = nil
	if err := sendBookingConfirmation(b, false); err != nil {
		t.Errorf("confirmation with nil mailer: %v", err)
	}
	if err := sendInvoiceEmail(sampleInvoiceView(db.InvoiceSent), []byte("%PDF-")); err != nil {
		t.Errorf("invoice with nil mailer: %v", err)
	}
	if err := sendReceiptEmail(sampleInvoiceView(db.InvoicePaid), []byte("%PDF-"), true); err != nil {
		t.Errorf("receipt with nil mailer: %v", err)
	}
	b.Email = ""
	if err := sendBookingCancellation(b, ""); err == nil {
		t.Error("expected error without an email address")
	}
}

func TestInvoicePDF(t *testing.T) {
	site = siteConfig{Name: businessName, Brand: brandName, BaseURL: "https://example.test", Email: "me@example.test", Phone: "0400 000 000",
		ABN: "12 345 678 901", BankName: "Up The Spout", BankBSB: "000-000", BankAcct: "12345678"}
	for _, status := range []string{db.InvoiceSent, db.InvoicePaid, db.InvoiceVoid} {
		out, err := invoicePDF(sampleInvoiceView(status))
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF-")) || len(out) < 1500 {
			t.Errorf("%s: pdf looks wrong (%d bytes)", status, len(out))
		}
	}
	// No bank details, no link, no booking: still renders.
	site.BankBSB = ""
	v := sampleInvoiceView(db.InvoiceDraft)
	v.Inv.PaymentLink, v.Booking = "", nil
	if _, err := invoicePDF(v); err != nil {
		t.Fatal(err)
	}
}

func TestMoneyHelpers(t *testing.T) {
	for in, want := range map[int64]string{0: "$0.00", 5: "$0.05", 12345: "$123.45", 123456789: "$1,234,567.89", -250: "-$2.50"} {
		if got := fmtCents(in); got != want {
			t.Errorf("fmtCents(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]int64{"": 0, "80": 8000, "129.5": 12950, "$1,200.99": 120099, "0.5": 50, "3.999": 399, "-58.00": -5800, "-$58.00": -5800, "$-58.00": -5800} {
		got, err := dollarsToCents(in)
		if err != nil || got != want {
			t.Errorf("dollarsToCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := dollarsToCents("abc"); err == nil {
		t.Error("expected error")
	}
	if fmtQty(4) != "4" || fmtQty(2.5) != "2.5" || fmtQty(0.25) != "0.25" {
		t.Error("fmtQty")
	}
	if fmtDuration(45) != "45 min" || fmtDuration(60) != "1 hr" || fmtDuration(90) != "1 hr 30 min" {
		t.Error("fmtDuration")
	}
	mon := startOfWeek(time.Date(2026, 8, 23, 15, 0, 0, 0, db.Melbourne)) // a Sunday
	if mon.Weekday() != time.Monday || mon.Day() != 17 || mon.Hour() != 0 {
		t.Errorf("startOfWeek = %v", mon)
	}
}

func TestRequireAdminCSRF(t *testing.T) {
	sessTok := "sess-" + generateSessionToken()
	userSessions.Store(sessTok, &userSession{User: &db.User{Email: "james67@gmail.com"}, Expiry: time.Now().Add(time.Hour), CSRF: "tok123"})
	defer userSessions.Delete(sessTok)
	called := false
	h := requireAdmin(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) })
	do := func(method, form, header string, cookie bool) int {
		called = false
		var body *strings.Reader
		if form != "" {
			body = strings.NewReader(form)
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(method, "/admin/x", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if header != "" {
			req.Header.Set("X-CSRF-Token", header)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: "user_session", Value: sessTok})
		}
		rr := httptest.NewRecorder()
		h(rr, req)
		return rr.Code
	}
	if code := do("GET", "", "", false); code != http.StatusSeeOther {
		t.Errorf("anonymous GET: %d", code)
	}
	if code := do("POST", "", "", false); code != http.StatusUnauthorized {
		t.Errorf("anonymous POST: %d", code)
	}
	if code := do("GET", "", "", true); code != 204 || !called {
		t.Errorf("admin GET: %d", code)
	}
	if code := do("POST", "", "", true); code != http.StatusForbidden || called {
		t.Errorf("POST without csrf: %d", code)
	}
	if code := do("POST", url.Values{"csrf": {"wrong"}}.Encode(), "", true); code != http.StatusForbidden || called {
		t.Errorf("POST wrong csrf: %d", code)
	}
	if code := do("POST", url.Values{"csrf": {"tok123"}, "x": {"1"}}.Encode(), "", true); code != 204 || !called {
		t.Errorf("POST form csrf: %d", code)
	}
	if code := do("POST", "", "tok123", true); code != 204 || !called {
		t.Errorf("POST header csrf: %d", code)
	}
	// Non-admin user is forbidden even for GET.
	other := "sess-" + generateSessionToken()
	userSessions.Store(other, &userSession{User: &db.User{Email: "someone@example.test"}, Expiry: time.Now().Add(time.Hour)})
	defer userSessions.Delete(other)
	req := httptest.NewRequest("GET", "/admin/x", nil)
	req.AddCookie(&http.Cookie{Name: "user_session", Value: other})
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin: %d", rr.Code)
	}
}
