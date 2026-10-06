package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestLlmsTxt(t *testing.T) {
	mux := seoTestSetup(t)
	rr := get(mux, "/llms.txt")
	if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("llms.txt: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "# Up The Spout\n") {
		t.Errorf("llms.txt must start with the H1:\n%.100s", body)
	}
	if !strings.Contains(body, "\n> ") && !strings.HasPrefix(body, "> ") {
		t.Error("llms.txt missing the blockquote summary")
	}
	// Pricing facts, service area and agent guidance must all be present.
	for _, want := range []string{
		"by Vin", "$219", "$289", "$389", "$489",
		"add $180", "$90 each", "12% off each", "$30 off each house", "Seniors Card holders: 20% off",
		"Donvale", "Warrandyte", "me@example.test",
		"no booking API", "service, property, guard (1), plan (1), issue, name, phone, email, preferred_time",
		"Property values: unit, single, large, double",
		"Do NOT include an address",
		"https://example.test/book?service=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("llms.txt missing %q", want)
		}
	}
	// Every property row and every service must be listed.
	for _, p := range propertyTypes {
		if !strings.Contains(body, fmt.Sprintf("%s (%s): $%d", p.Name, p.Note, p.Price)) {
			t.Errorf("llms.txt missing price row %q", p.Name)
		}
	}
	for i := range services {
		if !strings.Contains(body, services[i].Title) {
			t.Errorf("llms.txt missing service %q", services[i].Title)
		}
		if !strings.Contains(body, services[i].Slug) {
			t.Errorf("llms.txt missing slug %q", services[i].Slug)
		}
	}
	// Every internal link must resolve to a real 200 page (drift-proofing,
	// mirrors TestSitemap). Query strings are stripped before fetching.
	re := regexp.MustCompile(`https://example\.test(/[a-zA-Z0-9\-/]*)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		path := m[1]
		if seen[path] {
			continue
		}
		seen[path] = true
		if r := get(mux, path); r.Code != 200 {
			t.Errorf("llms.txt links to %s → %d", path, r.Code)
		}
	}
	// Pricing, the JSON API, areas, book, guides, plus every service page.
	if want := len(services) + 5; len(seen) < want {
		t.Errorf("expected at least %d internal links, got %d", want, len(seen))
	}

	// Optional figures appear only when configured.
	site.SeniorsPct, site.ABN = 0, ""
	if out := llmsTxt(); strings.Contains(out, "Seniors") || strings.Contains(out, "ABN") {
		t.Error("llms.txt mentions a seniors discount or ABN that isn't configured")
	}
	site.ABN = "12 345 678 901"
	if out := llmsTxt(); !strings.Contains(out, "ABN 12 345 678 901.") {
		t.Error("llms.txt missing the configured ABN")
	}
}

func TestIndexNowKey(t *testing.T) {
	mux := seoTestSetup(t) // seoTestSetup sets IndexNowKey before newMux registers the route
	rr := get(mux, "/"+site.IndexNowKey+".txt")
	if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("indexnow key file: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	if rr.Body.String() != site.IndexNowKey {
		t.Errorf("key file body = %q, want the bare key", rr.Body.String())
	}
}

func TestAPIPricing(t *testing.T) {
	mux := seoTestSetup(t)
	rr := get(mux, "/api/pricing")
	if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("api/pricing: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("api/pricing missing CORS header")
	}
	// The field names are a public contract that agents quote from.
	for _, key := range []string{`"currency"`, `"gst_included"`, `"property_types"`, `"plan_price"`, `"gutter_guard_add_on"`,
		`"downpipe_jetting_each"`, `"fire_ready_plan_discount_pct"`, `"neighbour_deal_off_each"`, `"seniors_discount_pct"`,
		`"every_clean_includes"`, `"services"`, `"service_area_suburbs"`, `"book_url"`, `"book_prefill_params"`} {
		if !strings.Contains(rr.Body.String(), key) {
			t.Errorf("api/pricing missing key %s", key)
		}
	}
	var p pricingResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("api/pricing not valid JSON: %v", err)
	}
	if p.Currency != "AUD" || p.GSTIncluded {
		t.Errorf("currency/gst wrong: %+v", p)
	}
	if p.GuardAddOn != 180 || p.DownpipeEach != 90 || p.PlanPct != 12 || p.NeighbourOff != 30 || p.SeniorsPct != 20 {
		t.Errorf("pricing numbers wrong: %+v", p)
	}
	if len(p.PropertyTypes) != len(propertyTypes) {
		t.Fatalf("property types: got %d, want %d", len(p.PropertyTypes), len(propertyTypes))
	}
	for i, got := range p.PropertyTypes {
		want := propertyTypes[i]
		if got.Slug != want.Slug || got.Name != want.Name || got.Price != want.Price || got.PlanPrice != want.PlanPrice() {
			t.Errorf("property type %d = %+v, want %+v", i, got, want)
		}
	}
	if single := p.PropertyTypes[1]; single.Slug != "single" || single.Price != 289 || single.PlanPrice != 254 {
		t.Errorf("single storey = %+v, want $289, or $254 on the plan", single)
	}
	if len(p.Includes) == 0 {
		t.Error("every_clean_includes is empty")
	}
	if len(p.Services) != len(services) {
		t.Errorf("services: got %d, want %d", len(p.Services), len(services))
	}
	if len(p.ServiceArea) != len(suburbs) {
		t.Errorf("service area: got %d, want %d", len(p.ServiceArea), len(suburbs))
	}
	if p.BookURL != "https://example.test/book" {
		t.Errorf("book_url = %q", p.BookURL)
	}
	if strings.Join(p.BookParams, ",") != "service,property,guard,plan,issue,name,phone,email,preferred_time" {
		t.Errorf("book_prefill_params = %v", p.BookParams)
	}
	for _, s := range p.Services {
		if !strings.HasPrefix(s.URL, "https://example.test/") {
			t.Errorf("service %s URL not absolute: %q", s.Slug, s.URL)
		}
	}
}

func TestBookPrefill(t *testing.T) {
	mux := seoTestSetup(t)

	// Valid params are echoed into the form, and the price shown matches them:
	// $489 double storey + $180 guard, less 12% on the plan = $589.
	body := get(mux, "/book?service=bushfire-preparation&property=double&guard=1&plan=1&name=Jane&phone=0400000000"+
		"&email=jane%40example.com&issue=Side+gate+code+1234&preferred_time=Tue+am").Body.String()
	for _, want := range []string{
		`value="Jane"`, `value="0400000000"`, `value="jane@example.com"`, `value="Tue am"`,
		`value="bushfire-preparation" selected`,
		`value="double" data-price="489" checked`,
		`name="guard" value="1" checked`, `name="plan" value="1" checked`,
		`id="price-total">$589<`,
		`>Side gate code 1234</textarea>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("prefilled form missing %s", want)
		}
	}

	// No (or an unknown) property starts on the default row with no extras.
	body = get(mux, "/book?property=mansion").Body.String()
	for _, want := range []string{`value="single" data-price="289" checked`, `id="price-total">$289<`} {
		if !strings.Contains(body, want) {
			t.Errorf("default form missing %s", want)
		}
	}
	if strings.Contains(body, `value="1" checked`) {
		t.Error("default form ticks guard or plan")
	}

	// Two service links imply an extra: the plan page ticks the plan, the
	// gutter guard page ticks the guard.
	if body = get(mux, "/book?service=fire-ready-plan").Body.String(); !strings.Contains(body, `name="plan" value="1" checked`) {
		t.Error("service=fire-ready-plan should tick the plan")
	}
	if body = get(mux, "/book?service=gutter-guard-homes").Body.String(); !strings.Contains(body, `name="guard" value="1" checked`) {
		t.Error("service=gutter-guard-homes should tick gutter guard")
	}

	// Unknown service slug is blanked — only the placeholder option is selected.
	body = get(mux, "/book?service=not-a-real-slug").Body.String()
	if !strings.Contains(body, `<option value="" selected>`) || strings.Contains(body, `"not-a-real-slug"`) {
		t.Error("unknown service slug should fall back to the empty option")
	}

	// Address params must be ignored: the address gate depends on autocomplete.
	body = get(mux, "/book?address=1+Evil+St&addr_street=1+Evil+St&addr_suburb=Donvale&addr_state=VIC&addr_postcode=3111").Body.String()
	if strings.Contains(body, "Evil") {
		t.Error("address query params must never be echoed")
	}
	for _, want := range []string{
		`name="addr_street" value=""`, `name="addr_suburb" value=""`,
		`name="addr_state" value=""`, `name="addr_postcode" value=""`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("hidden address field not empty: %s", want)
		}
	}

	// Over-length values are capped to the form's maxlength.
	long := strings.Repeat("a", 150)
	body = get(mux, "/book?name="+long).Body.String()
	if !strings.Contains(body, `value="`+strings.Repeat("a", 100)+`"`) || strings.Contains(body, strings.Repeat("a", 101)) {
		t.Error("name should be capped at 100 runes")
	}

	// Injected markup must render escaped, never as live HTML.
	probe := url.QueryEscape(`"><script>alert(1)</script>`)
	body = get(mux, "/book?name="+probe).Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("prefill value rendered unescaped")
	}
}
