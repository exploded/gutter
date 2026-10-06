package main

import (
	"strings"
	"testing"

	"gutter/db"
)

func TestMailTemplatesRender(t *testing.T) {
	site = siteConfig{Name: businessName, Owner: "Vin", BaseURL: "https://example.test", Email: "me@example.test", Phone: "0400 000 000"}
	b := &db.Booking{Name: "Ann <b>Bold</b>", Phone: "0400 111 222", Email: "ann@example.test",
		Suburb: "Donvale", Address: "12 Smith St, Donvale VIC 3111", ServiceSlug: "bushfire-preparation",
		PropertyType: "double", HasGuard: true, OnPlan: true,
		QuoteCents: int64(quoteDollars(propertyTypesBySlug["double"], true, true)) * 100,
		Issue:      "Side gate code 1234\nFriendly dog", PreferredTime: "Tomorrow arvo", IP: "1.2.3.4"}
	// $489 + $180 guard, less 12% for the plan, rounded to whole dollars.
	property, price := bookingSummary(b)
	if property != "Double-storey house, gutter guard fitted, Fire-ready plan" || price != "$589.00" {
		t.Errorf("bookingSummary = %q, %q", property, price)
	}
	for _, name := range []string{"booking-admin", "booking-customer"} {
		out, err := renderMail(name, map[string]any{"ID": int64(7), "B": b, "ServiceTitle": "Fire-season gutter clean",
			"Property": property, "Price": price, "Suspicious": true})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(out, "<b>Bold</b>") {
			t.Errorf("%s: user input not escaped", name)
		}
		for _, want := range []string{"https://example.test", "Ann", "Double-storey house", "$589.00",
			"Fire-season gutter clean", "Up The Spout — Vin"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q:\n%s", name, want, out)
			}
		}
	}
	if out, _ := renderMail("booking-customer", map[string]any{"B": b}); !strings.Contains(out, "<p>— Vin</p>") {
		t.Errorf("booking-customer: missing the owner's sign-off:\n%s", out)
	}
	// An admin-created booking has no price-list row and no price shown.
	if property, price := bookingSummary(&db.Booking{}); property != "" || price != "" {
		t.Errorf("empty bookingSummary = %q, %q", property, price)
	}
	// nil mailer is a no-op
	mail = nil
	notifyBooking(1, b, false)
}

func TestValidEmail(t *testing.T) {
	for in, want := range map[string]bool{
		"":                       false,
		"bob":                    false,
		"bob@example.test":       true,
		"Bob <bob@example.test>": false, // display-name form is not a plain address
		"a@b":                    true,
		"x@example.test\n":       false,
	} {
		if got := validEmail(in); got != want {
			t.Errorf("validEmail(%q) = %v, want %v", in, got, want)
		}
	}
}
