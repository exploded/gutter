package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gutter/db"
)

// TestBookingToInvoiceFlow drives the admin handlers end to end against a temp
// SQLite file: enquiry → customer linked → schedule → done → invoice draft →
// edit lines → send → public view/PDF → mark paid → follow-up booking.
func TestBookingToInvoiceFlow(t *testing.T) {
	if err := db.Open(filepath.Join(t.TempDir(), "flow.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var err error
	pages, err = loadTemplates("../../templates")
	if err != nil {
		t.Fatal(err)
	}
	site = siteConfig{Name: businessName, Owner: "Vin", BaseURL: "https://example.test", Email: "me@example.test",
		Prices: pricing(), SeniorsPct: 10, Suburbs: suburbs, Areas: suburbList,
		ABN: "12 345 678 901", BankName: "Warrandyte Gutters", BankBSB: "000-000", BankAcct: "12345678",
		ReviewURL: "https://g.page/r/TEST/review"}
	mail = nil

	sessTok := "sess-" + generateSessionToken()
	userSessions.Store(sessTok, &userSession{User: &db.User{Email: "james67@gmail.com"}, Expiry: time.Now().Add(time.Hour), CSRF: "csrf1"})
	defer userSessions.Delete(sessTok)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /book", handleBookSubmit)
	mux.HandleFunc("GET /invoice/{token}", handleInvoicePublic)
	mux.HandleFunc("GET /invoice/{token}/pdf", handleInvoicePublicPDF)
	mux.HandleFunc("GET /admin", requireAdmin(handleAdmin))
	mux.HandleFunc("GET /admin/bookings", requireAdmin(handleAdminBookings))
	mux.HandleFunc("GET /admin/bookings/new", requireAdmin(handleAdminBookingNew))
	mux.HandleFunc("POST /admin/bookings/new", requireAdmin(handleAdminBookingCreate))
	mux.HandleFunc("GET /admin/bookings/{id}", requireAdmin(handleAdminBooking))
	mux.HandleFunc("POST /admin/bookings/{id}/schedule", requireAdmin(handleAdminBookingSchedule))
	mux.HandleFunc("POST /admin/bookings/{id}/status", requireAdmin(handleAdminBookingStatus))
	mux.HandleFunc("POST /admin/bookings/{id}/notes", requireAdmin(handleAdminBookingNotes))
	mux.HandleFunc("POST /admin/bookings/{id}/issue", requireAdmin(handleAdminBookingIssue))
	mux.HandleFunc("POST /admin/bookings/{id}/followup", requireAdmin(handleAdminBookingFollowup))
	mux.HandleFunc("POST /admin/bookings/{id}/address", requireAdmin(handleAdminBookingAddress))
	mux.HandleFunc("GET /admin/calendar", requireAdmin(handleAdminCalendar))
	mux.HandleFunc("GET /admin/invoices", requireAdmin(handleAdminInvoices))
	mux.HandleFunc("POST /admin/invoices/new", requireAdmin(handleAdminInvoiceNew))
	mux.HandleFunc("GET /admin/invoices/{id}", requireAdmin(handleAdminInvoice))
	mux.HandleFunc("GET /admin/invoices/{id}/pdf", requireAdmin(handleAdminInvoicePDF))
	mux.HandleFunc("POST /admin/invoices/{id}/items", requireAdmin(handleAdminInvoiceItems))
	mux.HandleFunc("POST /admin/invoices/{id}/send", requireAdmin(handleAdminInvoiceSend))
	mux.HandleFunc("POST /admin/invoices/{id}/paid", requireAdmin(handleAdminInvoicePaid))
	mux.HandleFunc("POST /admin/invoices/{id}/void", requireAdmin(handleAdminInvoiceVoid))
	mux.HandleFunc("GET /admin/customers", requireAdmin(handleAdminCustomers))
	mux.HandleFunc("GET /admin/customers/new", requireAdmin(handleAdminCustomerNew))
	mux.HandleFunc("POST /admin/customers/new", requireAdmin(handleAdminCustomerCreate))
	mux.HandleFunc("GET /admin/customers/{id}", requireAdmin(handleAdminCustomer))
	mux.HandleFunc("POST /admin/customers/{id}", requireAdmin(handleAdminCustomerSave))
	mux.HandleFunc("POST /admin/customers/{id}/bookings", requireAdmin(handleAdminCustomerBooking))

	var extraCookies []*http.Cookie
	do := func(method, path string, form url.Values, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if form == nil && admin && method == "POST" {
			form = url.Values{}
		}
		if form != nil {
			if admin {
				form.Set("csrf", "csrf1")
			}
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.RemoteAddr = "203.0.113.5:1234"
		for _, c := range extraCookies {
			req.AddCookie(c)
		}
		if admin {
			req.AddCookie(&http.Cookie{Name: "user_session", Value: sessTok})
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	get := func(path string) string {
		t.Helper()
		rr := do("GET", path, nil, true)
		if rr.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}
	post := func(path string, form url.Values) string { // returns redirect location
		t.Helper()
		rr := do("POST", path, form, true)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("POST %s: %d %s", path, rr.Code, rr.Body.String())
		}
		loc := rr.Header().Get("Location")
		if strings.Contains(loc, "err=") {
			t.Fatalf("POST %s redirected with error: %s", path, loc)
		}
		return loc
	}

	// 1. Public enquiry (timestamp old enough to not be flagged). Without a
	// property type or a picked address (no addr_* fields) the form re-renders
	// with both errors and nothing is stored.
	rr := do("POST", "/book", url.Values{
		"name": {"Zoë O'Brien"}, "phone": {"0400 000 001"}, "email": {"Zoe@Example.test"},
		"address": {"12 Sample St typed by hand"}, "property": {"mansion"},
		"service": {"bushfire-preparation"}, "issue": {"Side gate code 1234. Friendly dog."},
		"preferred_time": {"Tue morning"}, "ts": {itoa(time.Now().Unix() - 10)},
	}, false)
	if body := rr.Body.String(); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "pick the address") ||
		!strings.Contains(body, "pick the type of property") {
		t.Fatalf("enquiry without property or picked address should 422 with both errors: %d", rr.Code)
	}
	if bs, _ := db.ListBookings(); len(bs) != 0 {
		t.Fatalf("rejected enquiry was stored: %+v", bs)
	}
	// This one arrives on an ad first, so the booking should end up carrying
	// both the source and the click behind it.
	landing := httptest.NewRecorder()
	trackSource(mux).ServeHTTP(landing, httptest.NewRequest("GET",
		"/?gclid=flow123&utm_medium=cpc&utm_campaign=42&utm_term=gutter+cleaning+warrandyte", nil))
	extraCookies = landing.Result().Cookies()

	rr = do("POST", "/book", url.Values{
		"name": {"Zoë O'Brien"}, "phone": {"0400 000 001"}, "email": {"Zoe@Example.test"},
		"address": {"12 Sample St, Donvale VIC 3111"}, "addr_street": {"12 Sample St"},
		"addr_suburb": {"Donvale"}, "addr_state": {"VIC"}, "addr_postcode": {"3111"},
		"property": {"double"}, "guard": {"1"}, "plan": {"1"},
		"service": {"bushfire-preparation"}, "issue": {"Side gate code 1234. Friendly dog."},
		"preferred_time": {"Tue morning"}, "ts": {itoa(time.Now().Unix() - 10)},
	}, false)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/book/thanks" {
		t.Fatalf("enquiry: %d %s", rr.Code, rr.Body.String())
	}
	bs, _ := db.ListBookings()
	if len(bs) != 1 || bs[0].CustomerID == 0 {
		t.Fatalf("enquiry not linked to a customer: %+v", bs)
	}
	b := bs[0]
	bid := b.ID
	if b.Source != db.SourceGoogleAds {
		t.Fatalf("enquiry source = %q, want %q", b.Source, db.SourceGoogleAds)
	}
	click, err := db.GetAdClickByBooking(bid)
	if err != nil || click == nil {
		t.Fatalf("ad click not linked to booking #%d: %+v, %v", bid, click, err)
	}
	if click.Keyword != "gutter cleaning warrandyte" || click.Campaign != "42" || click.GCLID != "flow123" {
		t.Fatalf("ad click = %+v", click)
	}
	// The click is spent: the cookie must not follow the visitor into a second
	// booking.
	if c := cookieByName(rr.Result().Cookies(), clickCookie); c == nil || c.MaxAge >= 0 {
		t.Errorf("booking left the click cookie = %+v, want it expired", c)
	}
	extraCookies = nil
	if b.Address != "12 Sample St, Donvale VIC 3111" || b.Suburb != "Donvale" {
		t.Fatalf("enquiry address not stored: %q / %q", b.Address, b.Suburb)
	}
	// The price-list choices are stored with the price the form showed
	// ($489 + $180 guard, less 12% on the plan) and the job length (150 min
	// for a double storey, plus 30 for the guard).
	if b.ServiceSlug != "bushfire-preparation" || b.PropertyType != "double" || !b.HasGuard || !b.OnPlan ||
		b.QuoteCents != 58900 || b.DurationMin != 180 || b.Status != db.BookingNew {
		t.Fatalf("enquiry price-list fields: %+v", b)
	}
	// The new customer's blank address is filled from the enquiry.
	if c, _ := db.GetCustomer(b.CustomerID); c.Address != "12 Sample St, Donvale VIC 3111" || c.Suburb != "Donvale" {
		t.Fatalf("customer address not filled from enquiry: %+v", c)
	}
	bpath := "/admin/bookings/" + itoa(bid)
	if l := get("/admin/bookings"); !strings.Contains(l, "Zoë") {
		t.Fatalf("bookings list missing content: %s", l)
	}
	if p := get(bpath); !strings.Contains(p, "Schedule the visit") {
		t.Fatalf("booking page missing content: %s", p)
	}
	// Address: hand-typed (no structured addr_* fields) is rejected; a picked
	// suggestion saves to the customer and syncs the booking's suburb.
	rr = do("POST", bpath+"/address", url.Values{"address": {"12 Nowhere St"}}, true)
	if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("hand-typed address should be rejected: %d %s", rr.Code, loc)
	}
	// Outside Victoria is outside the service area.
	rr = do("POST", bpath+"/address", url.Values{
		"address": {"1 Pitt St, Sydney NSW 2000"}, "addr_street": {"1 Pitt St"},
		"addr_suburb": {"Sydney"}, "addr_state": {"NSW"}, "addr_postcode": {"2000"},
	}, true)
	if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("interstate address should be rejected: %d %s", rr.Code, loc)
	}
	post(bpath+"/address", url.Values{
		"address": {"1 Example Ct, Doncaster East VIC 3109"}, "addr_street": {"1 Example Ct"},
		"addr_suburb": {"Doncaster East"}, "addr_state": {"VIC"}, "addr_postcode": {"3109"},
	})
	if nb, _ := db.GetBooking(bid); nb.Address != "1 Example Ct, Doncaster East VIC 3109" || nb.Suburb != "Doncaster East" {
		t.Fatalf("booking address not saved: %q / %q", nb.Address, nb.Suburb)
	}
	// The admin edit overwrites the customer's address too (invoices read it).
	if c, _ := db.GetCustomer(b.CustomerID); c.Address != "1 Example Ct, Doncaster East VIC 3109" || c.Suburb != "Doncaster East" {
		t.Fatalf("customer address not synced: %+v", c)
	}
	if rr := do("GET", "/admin", nil, true); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "1 click &rarr; 1 booking") {
		t.Fatalf("GET /admin: %d %.200s", rr.Code, rr.Body.String())
	}
	get("/admin/customers?q=zoe")

	// 2. Schedule via calendar-prefilled time, with confirmation email (nil mailer = ok).
	if !strings.Contains(get("/admin/calendar?for="+itoa(bid)), "?start=") {
		t.Fatal("calendar has no pick links")
	}
	loc := post(bpath+"/schedule", url.Values{"start": {"2026-08-25T09:30"}, "duration": {"75"}, "notify": {"1"}})
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("schedule: %s", loc)
	}
	b2, _ := db.GetBooking(bid)
	if b2.Status != db.BookingBooked || b2.DurationMin != 75 || fmtWhen(b2.StartAt) != "Tue 25 Aug, 9:30 am" {
		t.Fatalf("after schedule: %+v", b2)
	}
	if !strings.Contains(get("/admin/calendar?week=2026-08-25"), "9:30 am – 10:45 am") {
		t.Fatal("calendar does not show the event")
	}
	// Reschedule + notes + done.
	post(bpath+"/schedule", url.Values{"start": {"2026-08-25T14:00"}, "duration": {"60"}})
	post(bpath+"/notes", url.Values{"notes": {"Two downpipes jetted; rust on the back fascia."}})
	post(bpath+"/status", url.Values{"status": {"done"}})
	b2, _ = db.GetBooking(bid)
	if b2.Status != db.BookingDone || b2.AdminNotes == "" {
		t.Fatalf("after done: %+v", b2)
	}

	// 3. Invoice draft, prefilled from the booking's price-list choices; the
	// lines add up to the price the customer was shown.
	loc = post("/admin/invoices/new", url.Values{"booking": {itoa(bid)}})
	invPath := strings.SplitN(loc, "?", 2)[0]
	invID := lastSeg(invPath)
	page := get(invPath)
	for _, want := range []string{"INV-1000", "Gutter clean — Double-storey house", "Gutter guard lifted, cleaned under and refitted",
		"Fire-ready plan — 12% off", "$589.00", "+ Downpipe jetted ($90 each)", "+ Neighbour deal (&minus;$30)", "+ Seniors discount (&minus;10%)"} {
		if !strings.Contains(page, want) {
			t.Fatalf("draft page missing %q:\n%s", want, page)
		}
	}
	if inv, _ := db.GetInvoice(invID); inv.TotalCents != b.QuoteCents {
		t.Fatalf("prefilled total %d, want the quoted %d", inv.TotalCents, b.QuoteCents)
	}
	// Creating again reuses the draft.
	if loc2 := post("/admin/invoices/new", url.Values{"booking": {itoa(bid)}}); !strings.HasPrefix(loc2, invPath) {
		t.Fatalf("second create should reuse draft: %s", loc2)
	}
	// Edit: two downpipes jetted on the day and the neighbour deal (negative
	// unit), paste link.
	const editedTotal = 48900 + 18000 - 8000 + 2*9000 - 3000
	post(invPath+"/items", url.Values{
		"desc": {"Gutter clean — Double-storey house", "Gutter guard lifted, cleaned under and refitted", "Fire-ready plan — 12% off", "Downpipe jetted", "Neighbour deal", ""},
		"qty":  {"1", "1", "1", "2", "1", "1"},
		"unit": {"489", "180", "-80.00", "90", "-30.00", ""},
		"due":  {"2026-09-01"}, "notes": {"Thanks!"}, "payment_link": {"https://pay.example/zeller/abc"},
	})
	inv, _ := db.GetInvoice(invID)
	if inv.TotalCents != editedTotal || inv.PaymentLink != "https://pay.example/zeller/abc" || inv.Status != db.InvoiceDraft {
		t.Fatalf("after edit: %+v", inv)
	}
	// The editor renders the negative unit bare ("-30.00") and it must survive a
	// re-save unchanged (fmtCents-style "-$30.00" once round-tripped the editor).
	if pg := get(invPath); !strings.Contains(pg, `value="-30.00"`) || !strings.Contains(pg, "-$30.00") {
		t.Fatalf("discount line not rendered:\n%s", pg)
	}
	post(invPath+"/items", url.Values{
		"desc": {"Gutter clean — Double-storey house", "Gutter guard lifted, cleaned under and refitted", "Fire-ready plan — 12% off", "Downpipe jetted", "Neighbour deal", ""},
		"qty":  {"1", "1", "1", "2", "1", "1"},
		"unit": {"489", "180", "-$80.00", "90", "-$30.00", ""},
		"due":  {"2026-09-01"}, "notes": {"Thanks!"}, "payment_link": {"https://pay.example/zeller/abc"},
	})
	if inv, _ = db.GetInvoice(invID); inv.TotalCents != editedTotal {
		t.Fatalf("negative unit did not round-trip: %+v", inv)
	}
	// Bad link rejected.
	if rr := do("POST", invPath+"/items", url.Values{"csrf": {"csrf1"}, "desc": {"x"}, "qty": {"1"}, "unit": {"1"}, "payment_link": {"javascript:alert(1)"}}, true); !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatal("javascript link accepted")
	}

	// 4. Send → sent, booking invoiced; public page + PDFs work; resend allowed.
	post(invPath+"/send", nil)
	inv, _ = db.GetInvoice(invID)
	b2, _ = db.GetBooking(bid)
	if inv.Status != db.InvoiceSent || inv.IssuedAt.IsZero() || b2.Status != db.BookingInvoiced {
		t.Fatalf("after send: inv=%+v booking=%s", inv, b2.Status)
	}
	if rr := do("POST", invPath+"/items", url.Values{"csrf": {"csrf1"}, "desc": {"x"}, "qty": {"1"}, "unit": {"1"}}, true); !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatal("sent invoice should not be editable")
	}
	pub := do("GET", "/invoice/"+inv.ViewToken, nil, false)
	if pub.Code != 200 || !strings.Contains(pub.Body.String(), "INV-1000") || !strings.Contains(pub.Body.String(), "https://pay.example/zeller/abc") || !strings.Contains(pub.Body.String(), "$739.00") || !strings.Contains(pub.Body.String(), "-$30.00") {
		t.Fatalf("public invoice: %d\n%s", pub.Code, pub.Body.String())
	}
	if do("GET", "/invoice/"+strings.Repeat("b", 64), nil, false).Code != 404 || do("GET", "/invoice/short", nil, false).Code != 404 {
		t.Fatal("bad tokens should 404")
	}
	if p := do("GET", "/invoice/"+inv.ViewToken+"/pdf", nil, false); p.Code != 200 || !strings.HasPrefix(p.Body.String(), "%PDF-") || p.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("public pdf: %d", p.Code)
	}
	if p := do("GET", invPath+"/pdf", nil, true); p.Code != 200 {
		t.Fatalf("admin pdf: %d", p.Code)
	}
	post(invPath+"/send", nil) // resend
	get("/admin/invoices?status=sent")

	// 5. Mark paid with receipt → paid, booking paid; void refused afterwards.
	post(invPath+"/paid", url.Values{"method": {db.PayZellerLink}, "ref": {"Z-1"}, "notify": {"1"}})
	inv, _ = db.GetInvoice(invID)
	b2, _ = db.GetBooking(bid)
	if inv.Status != db.InvoicePaid || inv.PaymentMethod != db.PayZellerLink || b2.Status != db.BookingPaid {
		t.Fatalf("after paid: inv=%+v booking=%s", inv, b2.Status)
	}
	if rr := do("POST", invPath+"/void", url.Values{"csrf": {"csrf1"}}, true); !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatal("void of paid invoice should fail")
	}
	pub = do("GET", "/invoice/"+inv.ViewToken, nil, false)
	if !strings.Contains(pub.Body.String(), "Receipt") {
		t.Fatal("public page should show receipt")
	}
	// Paid: the receipt can be re-emailed; status stays paid.
	if page := get(invPath); !strings.Contains(page, "Resend receipt") {
		t.Fatal("paid invoice page should offer Resend receipt")
	}
	if loc := post(invPath+"/send", nil); !strings.Contains(loc, "Receipt") {
		t.Fatalf("resend receipt: %s", loc)
	}
	if inv, _ = db.GetInvoice(invID); inv.Status != db.InvoicePaid {
		t.Fatalf("resend receipt changed status: %s", inv.Status)
	}
	// The review ask is opt-in and recorded: untouched by the sends above, then
	// stamped when the box is ticked, after which the page stops offering it.
	if !inv.ReviewAskedAt.IsZero() {
		t.Fatalf("review asked without the tick-box: %v", inv.ReviewAskedAt)
	}
	if loc := post(invPath+"/send", url.Values{"review": {"1"}}); !strings.Contains(loc, "Review+request") {
		t.Fatalf("resend receipt with review: %s", loc)
	}
	if inv, _ = db.GetInvoice(invID); inv.ReviewAskedAt.IsZero() {
		t.Fatal("review ask was not recorded")
	}
	if page := get(invPath); !strings.Contains(page, "Review asked") || strings.Contains(page, "Ask for a Google review") {
		t.Fatal("paid invoice page should show the review as already asked")
	}

	// 6. Follow-up booking (the autumn plan clean) on the same customer,
	// unscheduled, carrying the same property, extras and price; its invoice
	// prefills to that price and numbers 1001.
	loc = post(bpath+"/followup", url.Values{"issue": {"Autumn plan clean"}})
	fid := lastSeg(strings.SplitN(loc, "?", 2)[0])
	fb, _ := db.GetBooking(fid)
	if fb.CustomerID != b.CustomerID || fb.ParentBookingID != bid || fb.Status != db.BookingNew || !fb.StartAt.IsZero() ||
		fb.PropertyType != "double" || !fb.HasGuard || !fb.OnPlan || fb.QuoteCents != 58900 || fb.Issue != "Autumn plan clean" {
		t.Fatalf("followup: %+v", fb)
	}
	loc = post("/admin/invoices/new", url.Values{"booking": {itoa(fid)}})
	inv2, _ := db.GetInvoice(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if inv2.Number != 1001 || inv2.TotalCents != 58900 {
		t.Fatalf("second invoice: number %d, total %d", inv2.Number, inv2.TotalCents)
	}
	post("/admin/invoices/"+itoa(inv2.ID)+"/void", nil)

	// 7. Customer edit and search.
	cpath := "/admin/customers/" + itoa(b.CustomerID)
	post(cpath, url.Values{"name": {"Zoë O'Brien"}, "email": {"zoe@example.test"}, "phone": {"0400 000 001"}, "address": {"1 Test St"}, "suburb": {"Donvale"}, "notes": {"Friendly dog."}})
	if !strings.Contains(get(cpath), "1 Test St") || !strings.Contains(get("/admin/customers?q=0400"), "Zoë") {
		t.Fatal("customer edit/search")
	}
	// New booking straight from the customer page: unscheduled, linked, uses saved details.
	loc = post(cpath+"/bookings", url.Values{"service": {"downpipe-unblocking"}, "issue": {"Back downpipe overflowing again"}})
	nid := lastSeg(strings.SplitN(loc, "?", 2)[0])
	nb, _ := db.GetBooking(nid)
	if nb == nil || nb.CustomerID != b.CustomerID || nb.Status != db.BookingNew || nb.ServiceSlug != "downpipe-unblocking" ||
		nb.Name != "Zoë O'Brien" || nb.Email != "zoe@example.test" || nb.Suburb != "Donvale" || !nb.StartAt.IsZero() {
		t.Fatalf("customer booking: %+v", nb)
	}
	if !strings.Contains(get(cpath), "/admin/bookings/"+itoa(nid)) {
		t.Fatal("customer page should list the new booking")
	}
	// Unknown service slug is dropped, not rejected.
	loc = post(cpath+"/bookings", url.Values{"service": {"nope"}, "issue": {"x"}})
	if nb, _ = db.GetBooking(lastSeg(strings.SplitN(loc, "?", 2)[0])); nb == nil || nb.ServiceSlug != "" {
		t.Fatalf("bad slug should be blanked: %+v", nb)
	}
	// Cancel the follow-up with an email (nil mailer).
	post("/admin/bookings/"+itoa(fid)+"/status", url.Values{"status": {"cancelled"}, "notify": {"1"}, "reason": {"house sold"}})
	if fb, _ = db.GetBooking(fid); fb.Status != db.BookingCancelled {
		t.Fatal("cancel")
	}
	// 8. Phone booking: /admin/bookings/new takes a caller's details directly.
	if p := get("/admin/bookings/new"); !strings.Contains(p, "Take a booking over the phone") {
		t.Fatalf("new-booking form missing content: %s", p)
	}
	// No name and no contact details → form re-renders with errors.
	if rr := do("POST", "/admin/bookings/new", url.Values{"csrf": {"csrf1"}, "issue": {"?"}}, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "caller&#39;s name") {
		t.Fatalf("empty phone booking should 422: %d %s", rr.Code, rr.Body.String())
	}
	// A hand-typed address (no picked addr_* fields) is rejected.
	if rr := do("POST", "/admin/bookings/new", url.Values{"csrf": {"csrf1"},
		"name": {"Rex Kramer"}, "phone": {"0400 000 099"}, "address": {"9 Typed St, Warranwood"},
	}, true); rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "pick the address") {
		t.Fatalf("hand-typed address should 422: %d", rr.Code)
	}
	// A brand-new caller with a picked address creates a new customer with the
	// address filled in; the booking carries the address, its suburb, and the
	// price-list choices made on the call ($389 large + $180 guard, 150 min).
	loc = post("/admin/bookings/new", url.Values{
		"name": {"Rex Kramer"}, "phone": {"0400 000 099"},
		"address": {"9 Sample St, Warranwood VIC 3134"}, "addr_street": {"9 Sample St"},
		"addr_suburb": {"Warranwood"}, "addr_state": {"VIC"}, "addr_postcode": {"3134"},
		"service": {"gutter-guard-homes"}, "property": {"large"}, "guard": {"1"},
		"issue": {"Gutters overflowing at the back"},
		"notes": {"Dog in the yard; use the side gate."},
	})
	pid := lastSeg(strings.SplitN(loc, "?", 2)[0])
	pb, _ := db.GetBooking(pid)
	if pb == nil || pb.CustomerID == 0 || pb.CustomerID == b.CustomerID || pb.Status != db.BookingNew ||
		pb.ServiceSlug != "gutter-guard-homes" || pb.Suburb != "Warranwood" ||
		pb.Address != "9 Sample St, Warranwood VIC 3134" || !pb.StartAt.IsZero() ||
		pb.PropertyType != "large" || !pb.HasGuard || pb.OnPlan || pb.QuoteCents != 56900 || pb.DurationMin != 150 ||
		pb.Source != db.SourcePhone || pb.AdminNotes != "Dog in the yard; use the side gate." {
		t.Fatalf("phone booking: %+v", pb)
	}
	// The notes can be corrected once the real job is known; they cannot be blanked.
	ppath := "/admin/bookings/" + itoa(pid)
	if rr := do("POST", ppath+"/issue", url.Values{"issue": {"  "}}, true); !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatalf("empty issue should be rejected: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	post(ppath+"/issue", url.Values{"issue": {"Guard is the old plastic type — brittle, refit with care"}})
	pb, _ = db.GetBooking(pid)
	if pb.Issue != "Guard is the old plastic type — brittle, refit with care" {
		t.Fatalf("issue not updated: %+v", pb)
	}
	if page := get(ppath); !strings.Contains(page, "Guard is the old plastic type") {
		t.Fatal("booking page does not show the edited notes")
	}
	if pc, _ := db.GetCustomer(pb.CustomerID); pc.Address != "9 Sample St, Warranwood VIC 3134" || pc.Suburb != "Warranwood" {
		t.Fatalf("phone booking customer address: %+v", pc)
	}
	// A repeat caller (same phone digits) is linked to the existing customer;
	// no address given is fine, a blank issue gets a placeholder, an unknown
	// property is dropped (no price, the default length), and the saved
	// customer address is not overwritten.
	loc = post("/admin/bookings/new", url.Values{"name": {"Zoë O'Brien"}, "phone": {"0400 000 001"}, "suburb": {"Donvale"},
		"property": {"mansion"}, "guard": {"1"}})
	pb, _ = db.GetBooking(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if pb == nil || pb.CustomerID != b.CustomerID || pb.Issue != "Phone enquiry" || pb.Address != "" ||
		pb.PropertyType != "" || pb.HasGuard || pb.QuoteCents != 0 || pb.DurationMin != 90 {
		t.Fatalf("repeat caller not linked: %+v", pb)
	}
	if pc, _ := db.GetCustomer(b.CustomerID); pc.Address != "1 Test St" {
		t.Fatalf("repeat caller should not change address: %+v", pc)
	}

	// 9. Invoice raised from the customer alone, with no booking behind it —
	// say a clean booked through a rental agency, started from a price-list row.
	if page := get(cpath); !strings.Contains(page, `name="customer" value="`+itoa(b.CustomerID)) ||
		!strings.Contains(page, `<option value="single">Single-storey house ($289)</option>`) {
		t.Fatal("customer page should offer New invoice with the price-list rows")
	}
	loc = post("/admin/invoices/new", url.Values{"customer": {itoa(b.CustomerID)}, "kind": {"single"}})
	sinv, _ := db.GetInvoice(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if sinv == nil || sinv.BookingID != 0 || sinv.CustomerID != b.CustomerID || sinv.Status != db.InvoiceDraft {
		t.Fatalf("standalone invoice: %+v", sinv)
	}
	sitems, _ := db.ListInvoiceItems(sinv.ID)
	if len(sitems) != 1 || sitems[0].Description != "Gutter clean — Single-storey house" || sitems[0].UnitCents != 28900 {
		t.Fatalf("single-storey seed lines: %+v", sitems)
	}
	spath := "/admin/invoices/" + itoa(sinv.ID)
	// Add two jetted downpipes, send it, and take payment — no booking status to mirror.
	post(spath+"/items", url.Values{
		"desc": {"Gutter clean — Single-storey house", "Downpipe jetted — rental at 4 Example Rd"}, "qty": {"1", "2"}, "unit": {"289", "90"},
		"due": {db.FormatDate(db.Today().AddDate(0, 0, 14))}, "payment_link": {"https://pay.example.test/abc"},
	})
	if sinv, _ = db.GetInvoice(sinv.ID); sinv.TotalCents != 28900+2*9000 {
		t.Fatalf("standalone total: %d", sinv.TotalCents)
	}
	post(spath+"/send", nil)
	post(spath+"/paid", url.Values{"method": {"bank_transfer"}, "notify": {"1"}})
	if sinv, _ = db.GetInvoice(sinv.ID); sinv.Status != db.InvoicePaid {
		t.Fatalf("standalone invoice status: %s", sinv.Status)
	}
	// The public view and PDF must not assume a booking.
	if rr := do("GET", "/invoice/"+sinv.ViewToken, nil, false); rr.Code != 200 ||
		!strings.Contains(rr.Body.String(), "rental at 4 Example Rd") {
		t.Fatalf("public standalone invoice: %d", rr.Code)
	}
	if rr := do("GET", "/invoice/"+sinv.ViewToken+"/pdf", nil, false); rr.Code != 200 || rr.Body.Len() < 1000 {
		t.Fatalf("standalone receipt PDF: %d %d bytes", rr.Code, rr.Body.Len())
	}
	// A blank draft starts with no lines at all.
	loc = post("/admin/invoices/new", url.Values{"customer": {itoa(b.CustomerID)}, "kind": {"blank"}})
	binv, _ := db.GetInvoice(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if items, _ := db.ListInvoiceItems(binv.ID); len(items) != 0 {
		t.Fatalf("blank seed lines: %+v", items)
	}
	// So does a kind the price list doesn't know (the old "software" kind).
	loc = post("/admin/invoices/new", url.Values{"customer": {itoa(b.CustomerID)}, "kind": {"software"}})
	if items, _ := db.ListInvoiceItems(lastSeg(strings.SplitN(loc, "?", 2)[0])); len(items) != 0 {
		t.Fatalf("unknown kind seed lines: %+v", items)
	}
	// Without a customer there is nothing to invoice.
	if rr := do("POST", "/admin/invoices/new", url.Values{}, true); rr.Code != http.StatusSeeOther ||
		!strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatalf("invoice with no customer should flash an error: %d", rr.Code)
	}

	// 10. Adding a customer by hand, with no booking anywhere in sight.
	if !strings.Contains(get("/admin/customers"), "/admin/customers/new") {
		t.Fatal("customers list should link to the new-customer form")
	}
	if p := get("/admin/customers/new"); !strings.Contains(p, "Add someone without a booking") {
		t.Fatalf("new-customer form missing content: %s", p)
	}
	// No name, and no contact details, are both rejected with the form re-rendered.
	if rr := do("POST", "/admin/customers/new", url.Values{"email": {"nobody@example.test"}}, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "enter the customer") {
		t.Fatalf("nameless customer should 422: %d", rr.Code)
	}
	if rr := do("POST", "/admin/customers/new", url.Values{"name": {"No Contact"}}, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "email address or a phone number") {
		t.Fatalf("contactless customer should 422: %d", rr.Code)
	}
	if rr := do("POST", "/admin/customers/new", url.Values{"name": {"Bad Email"}, "email": {"not-an-address"}}, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "look right") {
		t.Fatalf("bad email should 422: %d", rr.Code)
	}
	loc = post("/admin/customers/new", url.Values{"name": {"Ridge Property Agency"}, "email": {"Rentals@Example.test"}, "phone": {"0400 111 222"},
		"address": {"9 Main Rd"}, "suburb": {"Eltham"}, "notes": {"Invoice the agency; keys from the front desk."}})
	dev, _ := db.GetCustomer(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if dev == nil || dev.Name != "Ridge Property Agency" || dev.Email != "rentals@example.test" ||
		dev.Address != "9 Main Rd" || dev.Suburb != "Eltham" || dev.Notes != "Invoice the agency; keys from the front desk." {
		t.Fatalf("hand-added customer: %+v", dev)
	}
	// Re-adding the same person lands on their record instead of duplicating them.
	rr = do("POST", "/admin/customers/new", url.Values{"name": {"Ridge Agency Again"}, "email": {"rentals@example.test"}}, true)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "err=") ||
		lastSeg(strings.SplitN(rr.Header().Get("Location"), "?", 2)[0]) != dev.ID {
		t.Fatalf("duplicate customer should reopen the original: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	// Matching on phone digits alone counts too, however the number is spaced.
	rr = do("POST", "/admin/customers/new", url.Values{"name": {"Ridge Property Agency"}, "phone": {"0400111222"}}, true)
	if lastSeg(strings.SplitN(rr.Header().Get("Location"), "?", 2)[0]) != dev.ID {
		t.Fatalf("phone match should reopen the original: %s", rr.Header().Get("Location"))
	}
	// And the whole point: bill them, with no booking ever created.
	loc = post("/admin/invoices/new", url.Values{"customer": {itoa(dev.ID)}, "kind": {"large"}})
	dinv, _ := db.GetInvoice(lastSeg(strings.SplitN(loc, "?", 2)[0]))
	if dinv == nil || dinv.CustomerID != dev.ID || dinv.BookingID != 0 || dinv.TotalCents != 38900 {
		t.Fatalf("invoice for hand-added customer: %+v", dinv)
	}
	if bs, _ := db.ListBookingsByCustomer(dev.ID); len(bs) != 0 {
		t.Fatalf("hand-added customer should have no bookings: %+v", bs)
	}

	// Anonymous admin access is redirected; CSRF-less POST refused.
	if rr := do("GET", "/admin/bookings", nil, false); rr.Code != http.StatusSeeOther {
		t.Fatalf("anon admin: %d", rr.Code)
	}
	if rr := do("POST", bpath+"/notes", url.Values{"notes": {"x"}}, false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anon post: %d", rr.Code)
	}
}

var reID = regexp.MustCompile(`(\d+)$`)

func lastSeg(path string) int64 {
	m := reID.FindString(path)
	var n int64
	for _, c := range m {
		n = n*10 + int64(c-'0')
	}
	return n
}

func itoa(n int64) string { return fmtInt(n) }

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
