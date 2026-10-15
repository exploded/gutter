package main

import (
	"testing"

	"gutter/db"
)

// TestPriceList pins the published numbers. Every page, llms.txt, /api/pricing
// and the invoice prefill read them, so a change here should be deliberate.
func TestPriceList(t *testing.T) {
	want := []struct {
		slug         string
		price, mins  int
		plan, saving int
	}{
		{"unit", 190, 60, 167, 46},
		{"single", 235, 90, 207, 56},
		{"large", 350, 120, 308, 84},
		{"double", 395, 150, 348, 94},
	}
	if len(propertyTypes) != len(want) {
		t.Fatalf("got %d property types, want %d", len(propertyTypes), len(want))
	}
	for i, w := range want {
		p := &propertyTypes[i]
		if p.Slug != w.slug || p.Price != w.price || p.Minutes != w.mins || p.Name == "" || p.Note == "" {
			t.Errorf("row %d = %+v, want %s $%d %d min", i, *p, w.slug, w.price, w.mins)
		}
		if p.PlanPrice() != w.plan || p.PlanYear() != 2*w.plan || p.PlanSaving() != w.saving {
			t.Errorf("%s plan: %d/clean, %d/year, saves %d; want %d, %d, %d",
				p.Slug, p.PlanPrice(), p.PlanYear(), p.PlanSaving(), w.plan, 2*w.plan, w.saving)
		}
		if got, ok := findPropertyType(w.slug); !ok || got != p {
			t.Errorf("findPropertyType(%q) = %v, %v", w.slug, got, ok)
		}
	}
	if _, ok := findPropertyType(""); ok {
		t.Error("findPropertyType(\"\") should fail")
	}
	if _, ok := findPropertyType(defaultPropertyType); !ok {
		t.Errorf("default property type %q is not on the list", defaultPropertyType)
	}
	if got := priceRange(); got != "$190–$395" {
		t.Errorf("priceRange() = %q", got)
	}
	pi := pricing()
	if pi.Downpipe != 90 || pi.PlanPct != 12 || pi.Neighbour != 30 || pi.Referral != 20 ||
		pi.Range != "$190–$395" || len(pi.Types) != len(propertyTypes) {
		t.Errorf("pricing() = %+v", pi)
	}
}

func TestQuoteDollars(t *testing.T) {
	unit, single, double := propertyTypesBySlug["unit"], propertyTypesBySlug["single"], propertyTypesBySlug["double"]
	for _, c := range []struct {
		p    *PropertyType
		plan bool
		want int
	}{
		{single, false, 235},
		{single, true, 207}, // 235 × 0.88 = 206.8, rounded
		{double, true, 348}, // 395 × 0.88 = 347.6, rounded
		{unit, true, 167},   // 190 × 0.88 = 167.2
	} {
		if got := quoteDollars(c.p, c.plan); got != c.want {
			t.Errorf("quoteDollars(%s, plan=%v) = %d, want %d", c.p.Slug, c.plan, got, c.want)
		}
	}
}

// TestSeedInvoiceItems: a draft prefilled from a booking itemises exactly the
// price the customer was shown, for every combination on the list.
func TestSeedInvoiceItems(t *testing.T) {
	for _, p := range propertyTypes {
		for _, plan := range []bool{false, true} {
			b := &db.Booking{PropertyType: p.Slug, OnPlan: plan}
			items := seedInvoiceItems(b, "")
			var total int64
			for _, it := range items {
				total += db.LineCents(it.Qty, it.UnitCents)
			}
			if want := int64(quoteDollars(&p, plan)) * 100; total != want {
				t.Errorf("%s plan=%v: lines total %d, want %d: %+v", p.Slug, plan, total, want, items)
			}
			wantLines := 1
			if plan {
				wantLines++
			}
			if len(items) != wantLines || items[0].Description != "Gutter clean — "+p.Name {
				t.Errorf("%s plan=%v: lines %+v", p.Slug, plan, items)
			}
			if plan && (items[len(items)-1].Description != "Fire-ready plan — 12% off" || items[len(items)-1].UnitCents >= 0) {
				t.Errorf("%s: plan line %+v, want a negative 12%% off line", p.Slug, items[len(items)-1])
			}
		}
	}

	booked := &db.Booking{PropertyType: "double", OnPlan: true}
	// The customer page's kind wins over the booking, with no extras.
	if items := seedInvoiceItems(booked, "unit"); len(items) != 1 || items[0].UnitCents != 19000 {
		t.Errorf("kind=unit: %+v", items)
	}
	if items := seedInvoiceItems(nil, "large"); len(items) != 1 || items[0].UnitCents != 35000 {
		t.Errorf("kind=large, no booking: %+v", items)
	}
	// Nothing to go on starts a blank draft.
	for _, c := range []struct {
		name string
		b    *db.Booking
		kind string
	}{
		{"blank kind", booked, "blank"},
		{"unknown kind", nil, "software"},
		{"no booking, no kind", nil, ""},
		{"booking without a property", &db.Booking{OnPlan: true}, ""},
		{"booking with a stale property", &db.Booking{PropertyType: "mansion"}, ""},
	} {
		if items := seedInvoiceItems(c.b, c.kind); items != nil {
			t.Errorf("%s: %+v, want no lines", c.name, items)
		}
	}
}

// TestBookFormPrice: the booking page shows the price for the form's current
// choices before any script runs, and nothing for an unknown property.
func TestBookFormPrice(t *testing.T) {
	if got := (bookForm{PropertyType: "large", Plan: true}).Price(); got != 308 {
		t.Errorf("large on the plan = %d, want 308", got)
	}
	if got := (bookForm{PropertyType: "nope", Plan: true}).Price(); got != 0 {
		t.Errorf("unknown property = %d, want 0", got)
	}
}

// TestAdminEmails: ADMIN_EMAIL is a comma-separated list, and any address on
// it (in any case) gets into /admin.
func TestAdminEmails(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "")
	if got := adminEmails(); len(got) != 1 || got[0] != "james67@gmail.com" {
		t.Errorf("default adminEmails() = %v", got)
	}
	t.Setenv("ADMIN_EMAIL", " vin@example.test , James67@Gmail.com ,")
	if got := adminEmails(); len(got) != 2 || got[0] != "vin@example.test" || got[1] != "James67@Gmail.com" {
		t.Errorf("adminEmails() = %v", got)
	}
	for email, want := range map[string]bool{
		"vin@example.test":  true,
		"VIN@example.test":  true,
		"james67@gmail.com": true,
		"someone@else.test": false,
		"":                  false,
	} {
		if got := isAdmin(&db.User{Email: email}); got != want {
			t.Errorf("isAdmin(%q) = %v, want %v", email, got, want)
		}
	}
	if isAdmin(nil) {
		t.Error("isAdmin(nil) = true")
	}
}
