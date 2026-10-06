package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// ── /llms.txt ──
// AI assistants (Claude, ChatGPT, Perplexity, …) fetch /llms.txt to understand
// a site without scraping HTML. Everything here is built from the same live
// data as the pages themselves — services, the price list, site config,
// suburbs — so the file can never drift from the site. TestLlmsTxt also
// fetches every internal link it emits.

// llmsTxt builds the /llms.txt body (llmstxt.org format).
func llmsTxt() string {
	var b strings.Builder
	link := func(label, path string) string {
		return "[" + label + "](" + site.BaseURL + path + ")"
	}

	b.WriteString("# " + site.Name + "\n\n")
	b.WriteString("> Residential gutter and downpipe cleaning across Melbourne's north-east (based in Warrandyte VIC 3113), by " + site.Owner + ".\n")
	b.WriteString("> Fixed prices, online booking, and a dated before-and-after photo report with every clean.")
	if site.ABN != "" {
		b.WriteString(" ABN " + site.ABN + ".")
	}
	b.WriteString(" All prices in AUD; no GST is charged.\n\n")
	b.WriteString("Contact: " + site.Email)
	if site.Phone != "" {
		b.WriteString(" or " + site.Phone)
	}
	b.WriteString(". Hours " + strings.Join(site.HoursLD, "; ") + ".\n\n")

	b.WriteString("## Pricing (per clean, everything included)\n\n")
	for _, p := range propertyTypes {
		fmt.Fprintf(&b, "- %s (%s): $%d\n", p.Name, p.Note, p.Price)
	}
	fmt.Fprintf(&b, "- Gutter guard fitted: add $%d (lifted, cleaned underneath, refitted)\n", guardPrice)
	fmt.Fprintf(&b, "- Blocked downpipe that needs jetting: $%d each, only with the customer's OK\n", downpipePrice)
	fmt.Fprintf(&b, "- Fire-ready plan: two cleans a year (spring and May), %d%% off each\n", planPct)
	fmt.Fprintf(&b, "- Neighbour deal: $%d off each house when two or more on one street book the same day\n", neighbourOff)
	if site.SeniorsPct > 0 {
		fmt.Fprintf(&b, "- Seniors Card holders: %d%% off the total\n", site.SeniorsPct)
	}
	b.WriteString("- Every clean includes: all gutters and roof valleys cleared by hand, downpipes flushed, debris bagged and taken away, photo report.\n")
	b.WriteString("- Not offered: gutter or downpipe repairs (licensed plumbing work), gutter guard installation.\n")
	b.WriteString("- " + link("Pricing page", "/pricing") + " · machine-readable JSON: " + site.BaseURL + "/api/pricing\n\n")

	b.WriteString("## Services\n\n")
	for i := range services {
		s := &services[i]
		b.WriteString("- " + link(s.Title, s.URL()) + ": " + s.Short + "\n")
	}

	b.WriteString("\n## Service area\n\n")
	b.WriteString("These Victorian suburbs (base: Warrandyte VIC 3113):\n")
	b.WriteString(strings.Join(site.Suburbs, ", ") + ".\n")
	b.WriteString("- " + link("All areas", "/areas") + "\n\n")

	b.WriteString("## For AI assistants\n\n")
	b.WriteString("- To quote: pick the property row above, add the guard surcharge if gutter guard is fitted, and take the plan discount off if they join the Fire-ready plan.\n")
	b.WriteString("- There is no booking API. To help a user book, compose a prefilled link to " + site.BaseURL + "/book and give it to the user to open, review and submit themselves.\n")
	var slugs, types []string
	for i := range services {
		slugs = append(slugs, services[i].Slug)
	}
	for _, p := range propertyTypes {
		types = append(types, p.Slug)
	}
	b.WriteString("- Supported query parameters (all optional): service, property, guard (1), plan (1), issue, name, phone, email, preferred_time.\n")
	b.WriteString("- Service slugs: " + strings.Join(slugs, ", ") + "\n")
	b.WriteString("- Property values: " + strings.Join(types, ", ") + "\n")
	b.WriteString("- Do NOT include an address in the link — the form requires the user to pick their address from an autocomplete, and only Victorian addresses are accepted.\n")
	b.WriteString("- Example: " + site.BaseURL + "/book?service=bushfire-preparation&property=double&guard=1&preferred_time=weekday%20mornings\n\n")

	b.WriteString("## Optional\n\n")
	b.WriteString("- " + link("Gutter guides", "/guides") + "\n")
	return b.String()
}

func handleLlmsTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(llmsTxt()))
}

// ── GET /api/pricing ──

// pricingResponse is a stable public contract: AI agents quote from these
// numbers instead of parsing prose, so field names should not change lightly.
type pricingResponse struct {
	Currency      string        `json:"currency"`
	GSTIncluded   bool          `json:"gst_included"`
	PropertyTypes []apiProperty `json:"property_types"`
	GuardAddOn    int           `json:"gutter_guard_add_on"`
	DownpipeEach  int           `json:"downpipe_jetting_each"`
	PlanPct       int           `json:"fire_ready_plan_discount_pct"`
	NeighbourOff  int           `json:"neighbour_deal_off_each"`
	SeniorsPct    int           `json:"seniors_discount_pct"`
	Includes      []string      `json:"every_clean_includes"`
	Services      []apiService  `json:"services"`
	ServiceArea   []string      `json:"service_area_suburbs"`
	BookURL       string        `json:"book_url"`
	BookParams    []string      `json:"book_prefill_params"`
}

type apiProperty struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Note      string `json:"note"`
	Price     int    `json:"price"`
	PlanPrice int    `json:"plan_price"`
}

type apiService struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Short     string `json:"short"`
	PriceNote string `json:"price_note,omitempty"`
	URL       string `json:"url"`
}

func handleAPIPricing(w http.ResponseWriter, r *http.Request) {
	resp := pricingResponse{
		Currency:     "AUD",
		GuardAddOn:   guardPrice,
		DownpipeEach: downpipePrice,
		PlanPct:      planPct,
		NeighbourOff: neighbourOff,
		SeniorsPct:   site.SeniorsPct,
		Includes:     []string{"all gutters cleared by hand", "roof valleys cleared", "downpipes flushed", "debris bagged and taken away", "before-and-after photo report"},
		ServiceArea:  site.Suburbs,
		BookURL:      site.BaseURL + "/book",
		BookParams:   []string{"service", "property", "guard", "plan", "issue", "name", "phone", "email", "preferred_time"},
	}
	for i := range propertyTypes {
		p := &propertyTypes[i]
		resp.PropertyTypes = append(resp.PropertyTypes, apiProperty{Slug: p.Slug, Name: p.Name, Note: p.Note, Price: p.Price, PlanPrice: p.PlanPrice()})
	}
	for i := range services {
		s := &services[i]
		resp.Services = append(resp.Services, apiService{
			Slug: s.Slug, Title: s.Title, Short: s.Short, PriceNote: s.PriceNote, URL: site.BaseURL + s.URL(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("api/pricing: %v", err)
	}
}
