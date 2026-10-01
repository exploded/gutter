package main

import (
	"fmt"
	"math"
)

// The published price list. The pricing page, the booking form's live price,
// llms.txt, /api/pricing, the service pages and the invoice prefill all read
// from here, so changing a number here (and redeploying) changes it everywhere.

// PropertyType is one row of the price list. Customers pick one when booking;
// its slug is stored on the booking (bookings.property_type).
type PropertyType struct {
	Slug    string
	Name    string // "Single-storey house"
	Note    string // what counts as this size
	Price   int    // AUD for a full clean, everything included
	Minutes int    // typical time on site; the booking's default duration
}

var propertyTypes = []PropertyType{
	{Slug: "unit", Name: "Unit or townhouse", Note: "Single level, small roof", Price: 219, Minutes: 60},
	{Slug: "single", Name: "Single-storey house", Note: "Up to 4 bedrooms", Price: 289, Minutes: 90},
	{Slug: "large", Name: "Large single storey or split level", Note: "5+ bedrooms, or two roof levels", Price: 389, Minutes: 120},
	{Slug: "double", Name: "Double-storey house", Note: "Worked from a harness on a roof anchor", Price: 489, Minutes: 150},
}

const (
	guardPrice     = 180 // AUD: gutter guard lifted, gutters cleaned underneath, guard refitted
	downpipePrice  = 90  // AUD per blocked downpipe that needs jetting (agreed on site, never assumed)
	planPct        = 12  // Fire-ready plan: % off each of the two cleans a year
	neighbourOff   = 30  // AUD off each house when two or more on one street book the same day
	referralCredit = 20  // AUD off the next clean for both the referrer and the new customer
)

// defaultPropertyType is the row the booking form starts on.
const defaultPropertyType = "single"

var propertyTypesBySlug = map[string]*PropertyType{}

func init() {
	for i := range propertyTypes {
		propertyTypesBySlug[propertyTypes[i].Slug] = &propertyTypes[i]
	}
}

func findPropertyType(slug string) (*PropertyType, bool) {
	p, ok := propertyTypesBySlug[slug]
	return p, ok
}

// quoteDollars is the price of one clean: the base price, plus the guard
// surcharge, less the plan discount, rounded to whole dollars. It is the number
// the booking form shows and the booking records.
func quoteDollars(p *PropertyType, guard, plan bool) int {
	price := p.Price
	if guard {
		price += guardPrice
	}
	if plan {
		price = planPrice(price)
	}
	return price
}

// planPrice applies the Fire-ready plan discount to one clean.
func planPrice(price int) int {
	return int(math.Round(float64(price) * float64(100-planPct) / 100))
}

// PlanPrice is the per-clean plan price for this property (no guard).
func (p *PropertyType) PlanPrice() int { return planPrice(p.Price) }

// PlanYear is what two plan cleans cost over a year.
func (p *PropertyType) PlanYear() int { return 2 * p.PlanPrice() }

// PlanSaving is what the plan saves over two full-price cleans.
func (p *PropertyType) PlanSaving() int { return 2*p.Price - p.PlanYear() }

// priceRange renders the cheapest-to-dearest span of the list, e.g. "$219–$489".
func priceRange() string {
	lo, hi := propertyTypes[0].Price, propertyTypes[0].Price
	for _, p := range propertyTypes {
		lo, hi = min(lo, p.Price), max(hi, p.Price)
	}
	return fmt.Sprintf("$%d–$%d", lo, hi)
}

// pricingInfo is the price list as templates see it (.Site.Prices).
type pricingInfo struct {
	Types     []PropertyType
	Guard     int
	Downpipe  int
	PlanPct   int
	Neighbour int
	Referral  int
	Range     string
}

func pricing() pricingInfo {
	return pricingInfo{
		Types:     propertyTypes,
		Guard:     guardPrice,
		Downpipe:  downpipePrice,
		PlanPct:   planPct,
		Neighbour: neighbourOff,
		Referral:  referralCredit,
		Range:     priceRange(),
	}
}
