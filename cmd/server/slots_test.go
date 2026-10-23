package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"gutter/db"
)

// sampleSlotDays is a fixed picker for the template tests: a week of windows
// from a known Wednesday evening, with one taken.
func sampleSlotDays() []slotDay {
	now := time.Date(2026, 10, 14, 21, 0, 0, 0, db.Melbourne)
	var days []slotDay
	for _, s := range candidateSlots(now)[:6] {
		opt := slotOption{bookingSlot: s, Free: len(days) != 1}
		if n := len(days); n > 0 && days[n-1].Date.Day() == s.Start.Day() {
			days[n-1].Slots = append(days[n-1].Slots, opt)
			continue
		}
		days = append(days, slotDay{Date: s.Start, Slots: []slotOption{opt}})
	}
	return days
}

func TestWindowsOn(t *testing.T) {
	mel := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, db.Melbourne) }
	got := func(d int) []string {
		var out []string
		for _, s := range windowsOn(mel(d)) {
			out = append(out, s.Part+" "+s.Hours())
		}
		return out
	}
	for d, want := range map[int][]string{
		19: {"am 8am–12pm", "pm 12pm–5pm"}, // Monday
		24: {"am 8am–12pm"},                // Saturday: the afternoon is too short
		25: nil,                            // Sunday: closed
	} {
		if g := got(d); strings.Join(g, ",") != strings.Join(want, ",") {
			t.Errorf("windows on %d Oct = %v, want %v", d, g, want)
		}
	}
	if s := windowsOn(mel(19))[1]; s.Minutes() != 300 || s.Key() != "2026-10-19-pm" ||
		s.Label() != "Monday 19 October, afternoon (12pm–5pm)" || s.Short() != "Mon 19 Oct, afternoon" {
		t.Errorf("pm window = %+v %q %q", s, s.Label(), s.Short())
	}
}

func TestCandidateSlotsNotice(t *testing.T) {
	// Wednesday 9pm: tomorrow morning is only 11 hours away, so the first
	// window offered is tomorrow afternoon.
	now := time.Date(2026, 10, 14, 21, 0, 0, 0, db.Melbourne)
	cands := candidateSlots(now)
	if len(cands) == 0 || cands[0].Key() != "2026-10-15-pm" {
		t.Fatalf("first window = %+v, want 2026-10-15-pm", cands)
	}
	last := cands[len(cands)-1].Start
	if want := now.AddDate(0, 0, slotHorizonDays); last.After(want.Add(24 * time.Hour)) {
		t.Errorf("last window %v is past the horizon", last)
	}
	for _, s := range cands {
		if s.Start.Weekday() == time.Sunday {
			t.Errorf("offered a Sunday: %s", s.Key())
		}
	}
	if _, ok := slotForKey("2026-10-15-am", now); ok {
		t.Error("accepted a window inside the notice period")
	}
	if _, ok := slotForKey("2026-10-15-pm", now); !ok {
		t.Error("rejected a bookable window")
	}
	if _, ok := slotForKey("2027-01-04-am", now); ok {
		t.Error("accepted a window past the horizon")
	}
}

func TestOpenSlotsChecksCalendar(t *testing.T) {
	openTestDB(t, "slots-open.db")

	// Not connected: nothing can be checked, so nothing is offered.
	if days, ok := openSlots(time.Now()); ok || len(days) != 0 {
		t.Fatalf("offered %d day(s) without a calendar connection", len(days))
	}

	now := time.Now()
	cands := candidateSlots(now)
	// Pick a full weekday (two windows) for the busy block and the next one
	// for the booking.
	var weekdays []bookingSlot
	for i := 0; i+1 < len(cands); i++ {
		if cands[i].Part == db.SlotAM && cands[i+1].Part == db.SlotPM {
			weekdays = append(weekdays, cands[i], cands[i+1])
		}
	}
	if len(weekdays) < 4 {
		t.Fatal("not enough weekdays in the horizon")
	}
	amBusy, pmBusy, nextPM := weekdays[0], weekdays[1], weekdays[3]

	f := newFakeCalendar()
	f.cals = []gcalCalendar{{ID: "cal-app", Summary: calendarName, Role: "owner"}, {ID: "personal", Primary: true}}
	// A dentist appointment over midday blocks both windows of that day.
	f.busy = []busyInterval{{Start: amBusy.End.Add(-30 * time.Minute), End: pmBusy.Start.Add(30 * time.Minute)}}
	connectFake(t, f)
	// An existing booking from the app, which Google's free/busy never sees.
	id, err := db.InsertBooking(&db.Booking{Name: "Ann", DurationMin: 90})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ScheduleBooking(id, nextPM.Start.Add(time.Hour), 90); err != nil {
		t.Fatal(err)
	}

	days, ok := openSlots(now)
	if !ok || len(days) == 0 {
		t.Fatalf("openSlots = %d day(s), ok=%v", len(days), ok)
	}
	free := map[string]bool{}
	for _, d := range days {
		for _, o := range d.Slots {
			free[o.Key()] = o.Free
		}
	}
	if _, listed := free[amBusy.Key()]; listed {
		t.Errorf("fully busy day %s still listed", amBusy.Start.Format("2 Jan"))
	}
	if f, listed := free[nextPM.Key()]; !listed || f {
		t.Errorf("%s: listed=%v free=%v, want listed and taken", nextPM.Key(), listed, f)
	}
	if !free[weekdays[2].Key()] {
		t.Errorf("%s should be free", weekdays[2].Key())
	}
}

// TestBookSlotFlow books a window through the public form, then checks the
// same window can't be booked twice and the fallbacks hold.
func TestBookSlotFlow(t *testing.T) {
	openTestDB(t, "slots-flow.db")
	var err error
	if pages, err = loadTemplates("../../templates"); err != nil {
		t.Fatal(err)
	}
	site.Prices, site.Name, site.Brand = pricing(), businessName, brandName

	mux := http.NewServeMux()
	mux.HandleFunc("GET /book", handleBookForm)
	mux.HandleFunc("POST /book", handleBookSubmit)
	mux.HandleFunc("GET /book/thanks", handleBookThanks)

	n := 0
	post := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		n++
		base := url.Values{
			"name": {"Bea"}, "email": {"bea@example.test"}, "property": {"single"},
			"address": {"1 Sample St, Donvale VIC 3111"}, "addr_street": {"1 Sample St"},
			"addr_suburb": {"Donvale"}, "addr_state": {"VIC"}, "addr_postcode": {"3111"},
			"ts": {strconv.FormatInt(time.Now().Unix()-10, 10)},
		}
		for k, v := range form {
			base[k] = v
		}
		req := httptest.NewRequest("POST", "/book", strings.NewReader(base.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(n)) // stay under the rate limit
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	get := func(path string) string {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		return rr.Body.String()
	}

	slot := candidateSlots(time.Now())[2]

	// Not connected: the page offers no times, and a hand-made POST is
	// refused rather than booked unchecked.
	if body := get("/book"); strings.Contains(body, `name="slot"`) || !strings.Contains(body, `name="when" value="discuss"`) {
		t.Error("/book offered times without a calendar connection")
	}
	if rr := post(url.Values{"when": {"slot"}, "slot": {slot.Key()}}); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "couldn&#39;t check the calendar") {
		t.Fatalf("unchecked slot booking: %d", rr.Code)
	}

	f := newFakeCalendar()
	f.cals = []gcalCalendar{{ID: "personal", Primary: true}}
	connectFake(t, f)

	if body := get("/book"); !strings.Contains(body, `value="`+slot.Key()+`"`) {
		t.Fatalf("/book doesn't offer %s", slot.Key())
	}

	// Booking online needs an email for the confirmation.
	if rr := post(url.Values{"when": {"slot"}, "slot": {slot.Key()}, "email": {""}, "phone": {"0400 000 000"}}); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "booking confirmation") {
		t.Fatalf("slot booking without email: %d", rr.Code)
	}

	rr := post(url.Values{"when": {"slot"}, "slot": {slot.Key()}})
	want := "/book/thanks?day=" + slot.Start.Format("2006-01-02") + "&booked=" + slot.Part
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != want {
		t.Fatalf("slot booking: %d %q, want redirect to %q", rr.Code, rr.Header().Get("Location"), want)
	}
	bs, _ := db.ListBookings()
	if len(bs) != 1 {
		t.Fatalf("got %d booking(s), want 1", len(bs))
	}
	b := bs[0]
	if b.Status != db.BookingBooked || b.Slot != slot.Part || !b.StartAt.Equal(slot.Start) || b.DurationMin != slot.Minutes() {
		t.Fatalf("booked row = status %s slot %q start %v dur %d", b.Status, b.Slot, b.StartAt, b.DurationMin)
	}
	if !strings.Contains(get(want), slot.Label()) {
		t.Error("thanks page doesn't repeat the booked window")
	}

	// The window is gone from the picker, and a second customer still holding
	// the old page can't take it.
	busyCache.clear()
	if body := get("/book"); strings.Contains(body, `value="`+slot.Key()+`"`) {
		t.Error("booked window still offered")
	}
	if rr := post(url.Values{"when": {"slot"}, "slot": {slot.Key()}, "name": {"Cal"}}); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "just been taken") {
		t.Fatalf("double booking: %d", rr.Code)
	}

	// Google busy time added since the page loaded is caught by the fresh check.
	other := candidateSlots(time.Now())[4]
	f.busy = []busyInterval{{Start: other.Start, End: other.Start.Add(time.Hour)}}
	if rr := post(url.Values{"when": {"slot"}, "slot": {other.Key()}, "name": {"Dee"}}); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("booking over fresh busy time: %d", rr.Code)
	}

	// "Talk to us first" still sends a plain request, whatever slot was left
	// selected in the hidden picker.
	if rr := post(url.Values{"when": {"discuss"}, "slot": {other.Key()}, "preferred_time": {"Fridays"}, "name": {"Eve"}}); rr.Code != http.StatusSeeOther ||
		rr.Header().Get("Location") != "/book/thanks" {
		t.Fatalf("discuss request: %d %q", rr.Code, rr.Header().Get("Location"))
	}
	if bs, _ := db.ListBookings(); len(bs) != 2 || !bs[0].StartAt.IsZero() || bs[0].PreferredTime != "Fridays" {
		t.Fatalf("discuss request stored as %+v", bs[0])
	}

	// A suspiciously fast submission never takes a window.
	free := candidateSlots(time.Now())[6]
	if rr := post(url.Values{"when": {"slot"}, "slot": {free.Key()}, "name": {"Bot"}, "ts": {strconv.FormatInt(time.Now().Unix(), 10)}}); rr.Code != http.StatusSeeOther {
		t.Fatalf("suspicious submit: %d", rr.Code)
	}
	if bs, _ := db.ListBookings(); bs[0].Status != db.BookingSpam || !bs[0].StartAt.IsZero() {
		t.Fatalf("suspicious submit booked a window: %+v", bs[0])
	}
}
