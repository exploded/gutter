package main

import "fmt"

// Service is one entry in the service catalogue. The catalogue drives the home
// page grid, the /services index, and each /services/{slug} detail page.
type Service struct {
	Slug      string   // URL slug, e.g. "gutter-cleaning"
	Title     string   // "Gutter cleaning"
	Kicker    string   // short label shown above the card title
	Short     string   // 1–2 sentence card blurb
	Intro     string   // lead paragraph on the detail page
	Body      []string // further paragraphs
	Problems  []string // "Signs it's time" bullets
	PriceNote string   // service-specific pricing note
	Related   []string // slugs of related services
	MetaTitle string   // <title>
	MetaDesc  string   // meta description
}

// URL returns the canonical path for the service page.
func (s *Service) URL() string { return "/services/" + s.Slug }

// Number returns the zero-padded position of the service in the catalogue ("01".."05").
func (s *Service) Number() string {
	for i := range services {
		if services[i].Slug == s.Slug {
			return fmt.Sprintf("%02d", i+1)
		}
	}
	return "—"
}

var servicesBySlug = map[string]*Service{}

func init() {
	for i := range services {
		servicesBySlug[services[i].Slug] = &services[i]
	}
}

func findService(slug string) (*Service, bool) {
	s, ok := servicesBySlug[slug]
	return s, ok
}

func relatedServices(s *Service) []*Service {
	var out []*Service
	for _, slug := range s.Related {
		if r, ok := servicesBySlug[slug]; ok {
			out = append(out, r)
		}
	}
	return out
}

// single is the price-list row most copy quotes as the example.
var single = propertyTypesBySlugInit("single")

// propertyTypesBySlugInit looks a row up during package initialisation, before
// the init() that fills propertyTypesBySlug has run.
func propertyTypesBySlugInit(slug string) *PropertyType {
	for i := range propertyTypes {
		if propertyTypes[i].Slug == slug {
			return &propertyTypes[i]
		}
	}
	panic("pricing: no property type " + slug)
}

var services = []Service{
	{
		Slug:   "gutter-cleaning",
		Title:  "Gutter cleaning",
		Kicker: "Every clean",
		Short:  "Every gutter cleared with professional equipment, and the mess bagged and taken away — with a dated photo report.",
		Intro:  "A proper clean, done safely. We use professional gutter cleaning equipment, so no one has to risk their life on a ladder. We clear every metre of gutter and take everything away with us.",
		Body: []string{
			"Across Melbourne's leafy north-east, gum trees drop leaves, bark and gumnuts all year, so gutters fill faster than most people expect. Once they're full, water spills over the back edge and soaks the fascia and eaves. The timber rots, steel gutters rust through from the inside, and water finds its way into the roof, walls and footings. By then it's a job for a carpenter and a plumber, at many times the price of a clean.",
			"You get before-and-after photos of the gutters with your receipt, so you can see the work from the ground. Nothing is burnt and nothing is left in your garden beds.",
			"If we spot something that needs a plumber — a sagging section, a rusted-through joint, a broken downpipe — we'll photograph it and tell you. Repairs are licensed plumbing work in Victoria, so we don't do them.",
		},
		Problems: []string{
			"Water pours over the front of the gutter when it rains",
			"Grass, weeds or seedlings growing out of the gutter",
			"Stains, peeling paint or soft timber on the fascia and eaves",
			"Rust spots or water dripping through the bottom of the gutter",
			"Puddles or wet soil along the base of the walls",
			"It's been more than six months and there are gum trees nearby",
		},
		PriceNote: fmt.Sprintf("Fixed prices from $%d for a unit to $%d for a double storey. Taking the mess away is included.", propertyTypes[0].Price, propertyTypes[len(propertyTypes)-1].Price),
		Related:   []string{"bushfire-preparation", "fire-ready-plan", "downpipe-unblocking"},
		MetaTitle: "Gutter Cleaning in Melbourne's North-East — Fixed Prices, Book Online | Up The Spout",
		MetaDesc:  "Gutter cleaning in Melbourne's north-east: every gutter cleared with professional equipment, no ladders, photo report. Fixed prices, book online.",
	},
	{
		Slug:   "bushfire-preparation",
		Title:  "Fire-season gutter clean",
		Kicker: "Bushfire season",
		Short:  "A clean before the Fire Danger Period, timed to the CFA's advice, with a dated photo report you can show the council or your insurer.",
		Intro:  "Dry leaves in a gutter are exactly where embers land. The CFA tells people in bushfire areas to clear their gutters in early spring, before the Fire Danger Period is declared — and again through the season.",
		Body: []string{
			"This is our standard clean, booked for the weeks before the Fire Danger Period starts, with extra attention to the places embers collect: box gutters, the junction where a pergola or verandah meets the roof, and anything caught behind solar panels that can be safely reached.",
			"Your photo report is dated, so it's a record that the work was done. If you've received a Fire Prevention Notice from Manningham or Nillumbik council, send us the deadline when you book and we'll fit you in before it.",
			"A clean gutter is one part of a bushfire plan, not the whole of it. The CFA's property preparation guide covers the rest.",
		},
		Problems: []string{
			"You live in or near the Bushfire Management Overlay",
			"You've had a Fire Prevention Notice from the council",
			"Leaves and bark are sitting in the gutters going into summer",
			"You want a dated record of the work for your insurer",
		},
		PriceNote: "Same fixed prices as a standard clean. Book early: October to December is the busiest time of the year.",
		Related:   []string{"fire-ready-plan", "gutter-cleaning", "downpipe-unblocking"},
		MetaTitle: "Bushfire Season Gutter Cleaning — Melbourne's North-East | Up The Spout",
		MetaDesc:  "Get your gutters fire-ready before the Fire Danger Period. Dated photo report for council notices and insurers. Fixed prices, book online.",
	},
	{
		Slug:   "fire-ready-plan",
		Title:  "Fire-ready plan",
		Kicker: "Twice a year",
		Short:  fmt.Sprintf("Two cleans a year — one before fire season, one after the autumn leaf drop — at %d%% off each. We remind you, you just say yes.", planPct),
		Intro:  "Gum-tree properties need their gutters done more than once a year. The plan books you in twice: in spring before the Fire Danger Period, and in May after the autumn leaf fall.",
		Body: []string{
			fmt.Sprintf("Each plan clean is %d%% off the normal price. For a single-storey house that's $%d a clean, or $%d a year instead of $%d.", planPct, single.PlanPrice(), single.PlanYear(), 2*single.Price),
			"About a month before each clean we'll email you to pick a day. There's no contract and nothing is charged in advance: you pay after each clean, and you can drop out any time.",
		},
		Problems: []string{
			"Tall gums or stringybarks overhang the roof",
			"You'd rather not have to remember",
			"Your gutters were overflowing within a few months of the last clean",
		},
		PriceNote: fmt.Sprintf("%d%% off every plan clean. Tick \"Fire-ready plan\" when you book.", planPct),
		Related:   []string{"bushfire-preparation", "gutter-cleaning"},
		MetaTitle: "Fire-Ready Gutter Plan — Two Cleans a Year | Up The Spout",
		MetaDesc:  fmt.Sprintf("Spring and autumn gutter cleans at %d%% off each, with reminders. No contract, pay after each clean. Across Melbourne's north-east.", planPct),
	},
	{
		Slug:   "downpipe-unblocking",
		Title:  "Blocked downpipes",
		Kicker: "Downpipes",
		Short:  "If a clean turns up a blocked downpipe, we'll tell you, and where it can be reached we can clear it with a jetter — only with your OK.",
		Intro:  "A clear gutter is no use if the downpipe below it is blocked. Leaves and gumnuts wash down and pack into the bends, and the next heavy rain backs up and overflows.",
		Body: []string{
			"Downpipe flushing isn't part of the standard clean: on some properties, especially taller ones, the downpipes can't be reached. If we see signs of a blockage, we'll point it out and photograph it.",
			"Where a blocked downpipe can be reached, we can clear it with a drain jetter, with your OK. If the problem is underground — a collapsed or root-filled stormwater pipe — that's one for a licensed plumber, and we'll tell you so.",
		},
		Problems: []string{
			"One section of gutter overflows while the rest are fine",
			"Water gushes out of a joint in the downpipe",
			"A gurgling downpipe, or water pooling at its base",
		},
		PriceNote: fmt.Sprintf("A blocked downpipe that needs jetting is $%d, and only with your OK.", downpipePrice),
		Related:   []string{"gutter-cleaning", "bushfire-preparation"},
		MetaTitle: "Blocked Downpipe Clearing — Melbourne's North-East | Up The Spout",
		MetaDesc:  "Blocked downpipes cleared with a jetter, with your OK, where they can be reached. Fixed prices across Melbourne's north-east.",
	},
	{
		Slug:   "pre-sale-and-rentals",
		Title:  "Selling or renting out",
		Kicker: "Property",
		Short:  "A clean before the photos, the building inspection or a new tenant — with a photo report for the file.",
		Intro:  "Building inspectors note blocked gutters, and buyers notice them on inspection day. Rentals need them done too, and a dated record of the work helps if there's ever a dispute about water damage.",
		Body: []string{
			"For sellers we'll fit the clean in before the photographer or the building inspector arrives. For landlords and property managers we can collect keys from the agency, invoice the agency or owner directly, and send the photo report for your records.",
		},
		Problems: []string{
			"A building inspection or open for inspection is coming up",
			"A tenant has reported overflowing gutters",
			"You manage a rental and need a dated record of maintenance",
		},
		PriceNote: "Same fixed prices. Invoices can go to the owner or the agency.",
		Related:   []string{"gutter-cleaning", "downpipe-unblocking"},
		MetaTitle: "Pre-Sale & Rental Gutter Cleaning — Melbourne's North-East | Up The Spout",
		MetaDesc:  "Gutter cleans before a sale, inspection or new tenancy, with a dated photo report. Invoice the owner or the agency. Fixed prices, book online.",
	},
}
