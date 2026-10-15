package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gutter/db"
)

func TestTimeOffSpan(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, db.Melbourne)
	at := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, db.Melbourne) }
	for _, c := range []struct {
		name       string
		f          timeOffForm
		start, end time.Time
		err        string
	}{
		{"whole day", timeOffForm{Date: "2026-10-20"}, at(20, 0, 0), at(21, 0, 0), ""},
		{"morning", timeOffForm{Date: "2026-10-20", From: "08:00", To: "12:00"}, at(20, 8, 0), at(20, 12, 0), ""},
		{"from 2pm", timeOffForm{Date: "2026-10-20", From: "14:00"}, at(20, 14, 0), at(21, 0, 0), ""},
		{"several days", timeOffForm{Date: "2026-10-20", Until: "2026-10-23"}, at(20, 0, 0), at(24, 0, 0), ""},
		{"overnight", timeOffForm{Date: "2026-10-20", Until: "2026-10-21", From: "14:00", To: "10:00"}, at(20, 14, 0), at(21, 10, 0), ""},
		{"no day", timeOffForm{}, time.Time{}, time.Time{}, "Choose the day"},
		{"backwards days", timeOffForm{Date: "2026-10-20", Until: "2026-10-19"}, time.Time{}, time.Time{}, "before the first"},
		{"backwards times", timeOffForm{Date: "2026-10-20", From: "12:00", To: "08:00"}, time.Time{}, time.Time{}, "after the start"},
		{"too long", timeOffForm{Date: "2026-10-20", Until: "2027-03-01"}, time.Time{}, time.Time{}, "more than"},
		{"past", timeOffForm{Date: "2026-10-14"}, time.Time{}, time.Time{}, "already passed"},
		{"bad time", timeOffForm{Date: "2026-10-20", From: "9am"}, time.Time{}, time.Time{}, "valid time"},
	} {
		start, end, msg := c.f.span(now)
		if c.err != "" {
			if !strings.Contains(msg, c.err) {
				t.Errorf("%s: message %q, want it to mention %q", c.name, msg, c.err)
			}
			continue
		}
		if msg != "" || !start.Equal(c.start) || !end.Equal(c.end) {
			t.Errorf("%s: got %v – %v (%q), want %v – %v", c.name, start, end, msg, c.start, c.end)
		}
	}
}

func TestFmtTimeOff(t *testing.T) {
	at := func(mo time.Month, d, h int) time.Time { return time.Date(2026, mo, d, h, 0, 0, 0, db.Melbourne) }
	for _, c := range []struct {
		t    timeOff
		want string
	}{
		{timeOff{Start: at(10, 20, 0), End: at(10, 21, 0)}, "Tue 20 Oct, all day"},
		{timeOff{Start: at(10, 20, 0), End: at(10, 24, 0)}, "Tue 20 – Fri 23 Oct"},
		{timeOff{Start: at(10, 30, 0), End: at(11, 3, 0)}, "Fri 30 Oct – Mon 2 Nov"},
		{timeOff{Start: at(10, 20, 9), End: at(10, 20, 13)}, "Tue 20 Oct, 9:00 am – 1:00 pm"},
		{timeOff{Start: at(10, 20, 14), End: at(10, 21, 10)}, "Tue 20 Oct, 2:00 pm – Wed 21 Oct, 10:00 am"},
	} {
		if got := fmtTimeOff(c.t); got != c.want {
			t.Errorf("fmtTimeOff = %q, want %q", got, c.want)
		}
	}
}

// TestTimeOffFromEvents checks which events on the app's calendar count as
// time off, and that the form's events read back as they were written.
func TestTimeOffFromEvents(t *testing.T) {
	site = siteConfig{BaseURL: "https://example.test"}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, db.Melbourne)
	allDay := eventForTimeOff(timeOff{Start: day, End: day.AddDate(0, 0, 2), Note: "Holiday"})
	timed := eventForTimeOff(timeOff{Start: day.Add(9 * time.Hour), End: day.Add(13 * time.Hour)})
	if allDay.Start.Date != "2026-10-20" || allDay.End.Date != "2026-10-22" || allDay.Start.DateTime != "" {
		t.Errorf("all-day event times = %+v / %+v", allDay.Start, allDay.End)
	}
	if timed.Start.DateTime != "2026-10-20T09:00:00" || timed.Start.Date != "" || timed.Summary != "Time off" {
		t.Errorf("timed event = %q %+v", timed.Summary, timed.Start)
	}
	// Google answers with an offset, not the zone name the app sends.
	timed.Start = &gcalEventTime{DateTime: "2026-10-20T09:00:00+11:00"}
	timed.End = &gcalEventTime{DateTime: "2026-10-20T13:00:00+11:00"}

	booking := eventForBooking(&db.Booking{ID: 7, Name: "Ann", StartAt: day.Add(14 * time.Hour), DurationMin: 90})
	booking.Start = &gcalEventTime{DateTime: "2026-10-20T14:00:00+11:00"}
	booking.End = &gcalEventTime{DateTime: "2026-10-20T15:30:00+11:00"}
	phone := gcalEvent{ID: "p", Summary: "School concert",
		Start: &gcalEventTime{DateTime: "2026-10-21T15:00:00+11:00"}, End: &gcalEventTime{DateTime: "2026-10-21T17:00:00+11:00"}}
	free := phone
	free.ID, free.Transparency = "f", "transparent"

	got := timeOffFromEvents([]gcalEvent{*allDay, *timed, *booking, phone, free})
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3 (booking and free event skipped): %+v", len(got), got)
	}
	if got[0].Note != "Holiday" || !got[0].AllDay() || !got[0].Start.Equal(day) || !got[0].End.Equal(day.AddDate(0, 0, 2)) {
		t.Errorf("all-day span = %+v", got[0])
	}
	if got[1].Note != "" || got[1].AllDay() || !got[1].Start.Equal(day.Add(9*time.Hour)) {
		t.Errorf("timed span = %+v", got[1])
	}
	if got[2].Note != "School concert" {
		t.Errorf("phone event note = %q, want its title", got[2].Note)
	}
}

// TestTimeOffFollowsGoogle blocks out a morning, then moves and deletes the
// event "in Google" and checks the online picker follows each change.
func TestTimeOffFollowsGoogle(t *testing.T) {
	openTestDB(t, "timeoff-slots.db")
	f := newFakeCalendar()
	f.cals = []gcalCalendar{{ID: "cal-app", Summary: calendarName, Role: "owner"}}
	connectFake(t, f)

	now := time.Now()
	cands := candidateSlots(now)
	var am, pm bookingSlot
	for i := 0; i+1 < len(cands); i++ {
		if cands[i].Part == db.SlotAM && cands[i+1].Part == db.SlotPM {
			am, pm = cands[i], cands[i+1]
			break
		}
	}
	if am.Start.IsZero() {
		t.Fatal("no full weekday in the horizon")
	}
	free := func() map[string]bool {
		t.Helper()
		days, ok := openSlots(now)
		if !ok {
			t.Fatal("openSlots not ok")
		}
		out := map[string]bool{}
		for _, d := range days {
			for _, o := range d.Slots {
				out[o.Key()] = o.Free
			}
		}
		return out
	}

	// The dentist at 11, added from the app: the morning goes.
	f.events["dentist"] = eventForTimeOff(timeOff{Start: am.Start.Add(3 * time.Hour), End: am.End, Note: "Dentist"})
	if fr := free(); fr[am.Key()] || !fr[pm.Key()] {
		t.Errorf("with time off: am free=%v pm free=%v, want am taken, pm free", fr[am.Key()], fr[pm.Key()])
	}

	// Moved to the afternoon in Google. The final check reads Google afresh;
	// the picker catches up once its cache entry expires.
	f.events["dentist"] = eventForTimeOff(timeOff{Start: pm.Start, End: pm.Start.Add(time.Hour), Note: "Dentist"})
	if ok, err := slotStillFree(am); err != nil || !ok {
		t.Errorf("after the move, slotStillFree(am) = %v, %v; want free", ok, err)
	}
	if ok, _ := slotStillFree(pm); ok {
		t.Error("after the move, the afternoon is still offered")
	}
	timeOffCache.clear()
	if fr := free(); !fr[am.Key()] || fr[pm.Key()] {
		t.Errorf("picker after the move: am free=%v pm free=%v", fr[am.Key()], fr[pm.Key()])
	}

	// Deleted in Google: both windows open again.
	delete(f.events, "dentist")
	if ok, _ := slotStillFree(pm); !ok {
		t.Error("after deleting it in Google, the afternoon is still taken")
	}

	// Google unreachable: no windows offered, rather than offering unchecked ones.
	f.listErr = errors.New("boom")
	timeOffCache.clear()
	if days, ok := openSlots(now); ok || len(days) != 0 {
		t.Errorf("time off unreadable but %d day(s) offered", len(days))
	}
}

// TestTimeOffHandlers drives the form: a bad entry comes back as the form
// fragment with the typed values, a good one lands in Google and redirects to
// its week, the calendar shows it, and Remove deletes the event.
func TestTimeOffHandlers(t *testing.T) {
	openTestDB(t, "timeoff-http.db")
	var err error
	if pages, err = loadTemplates("../../templates"); err != nil {
		t.Fatal(err)
	}
	site = siteConfig{Name: businessName, Brand: brandName, BaseURL: "https://example.test", Email: "me@example.test", Prices: pricing(), Suburbs: suburbs}
	sessTok := "sess-" + generateSessionToken()
	userSessions.Store(sessTok, &userSession{User: &db.User{Email: "james67@gmail.com"}, Expiry: time.Now().Add(time.Hour), CSRF: "csrf1"})
	defer userSessions.Delete(sessTok)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/calendar", requireAdmin(handleAdminCalendar))
	mux.HandleFunc("POST /admin/calendar/off", requireAdmin(handleAdminTimeOffCreate))
	mux.HandleFunc("POST /admin/calendar/off/{id}/delete", requireAdmin(handleAdminTimeOffDelete))
	do := func(method, path string, form url.Values, htmx bool) *httptest.ResponseRecorder {
		t.Helper()
		var req *http.Request
		if form != nil {
			form.Set("csrf", "csrf1")
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: "user_session", Value: sessTok})
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	day := db.Today().AddDate(0, 0, 8)
	date := day.Format("2006-01-02")
	good := url.Values{"date": {date}, "from": {"09:00"}, "to": {"13:00"}, "note": {"Dentist"}}

	// Not connected: the page says so, and a post is turned away.
	if body := do("GET", "/admin/calendar", nil, false).Body.String(); strings.Contains(body, `id="time-off-form"`) ||
		!strings.Contains(body, "Connect Google Calendar") {
		t.Error("disconnected calendar page still offers the form")
	}
	if rr := do("POST", "/admin/calendar/off", url.Values{"date": {date}}, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "connect it") {
		t.Errorf("disconnected post: %d %s", rr.Code, rr.Body.String())
	}

	f := newFakeCalendar()
	connectFake(t, f)

	// Backwards times: 422 with the form, the message and the typed note.
	rr := do("POST", "/admin/calendar/off", url.Values{"date": {date}, "from": {"13:00"}, "to": {"09:00"}, "note": {"Dentist"}}, true)
	body := rr.Body.String()
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `id="time-off-form"`) ||
		!strings.Contains(body, "after the start") || !strings.Contains(body, `value="Dentist"`) {
		t.Fatalf("bad entry: %d %s", rr.Code, body)
	}
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("bad entry returned the whole page, want the form fragment")
	}

	// Google refuses: the form comes back with the reason, nothing is lost.
	f.insertErr = errors.New("quota exceeded")
	if rr := do("POST", "/admin/calendar/off", good, true); rr.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rr.Body.String(), "quota exceeded") {
		t.Errorf("insert failure: %d %s", rr.Code, rr.Body.String())
	}

	// A good one goes into Google and redirects to its week via HX-Redirect.
	rr = do("POST", "/admin/calendar/off", good, true)
	loc := rr.Header().Get("HX-Redirect")
	if rr.Code != http.StatusOK || !strings.Contains(loc, "week="+startOfWeek(day).Format("2006-01-02")) || !strings.Contains(loc, "ok=") {
		t.Fatalf("good entry: %d HX-Redirect=%q", rr.Code, loc)
	}
	if f.inserts != 1 {
		t.Fatalf("inserts = %d, want 1", f.inserts)
	}
	var id string
	for k, ev := range f.events {
		id = k
		if ev.Summary != "Time off: Dentist" || ev.Start.DateTime != date+"T09:00:00" {
			t.Errorf("event = %q %+v", ev.Summary, ev.Start)
		}
	}

	// The calendar draws it and lists it with a Remove button.
	body = do("GET", "/admin/calendar?week="+date, nil, false).Body.String()
	if !strings.Contains(body, `class="cal-off"`) || !strings.Contains(body, "Time off 9:00 am – 1:00 pm: Dentist") ||
		!strings.Contains(body, "/admin/calendar/off/"+id+"/delete") {
		t.Errorf("calendar page is missing the time off")
	}

	// A booking's event can't be removed from here.
	bid, _ := db.InsertBooking(&db.Booking{Name: "Ann"})
	f.events["booking-ev"] = eventForBooking(&db.Booking{ID: bid, Name: "Ann", StartAt: day.Add(14 * time.Hour), DurationMin: 60})
	if rr := do("POST", "/admin/calendar/off/booking-ev/delete", url.Values{}, false); !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Errorf("removing a booking event: %q, want an error", rr.Header().Get("Location"))
	}
	if _, ok := f.events["booking-ev"]; !ok {
		t.Error("booking event was deleted")
	}

	// Remove deletes the Google event.
	rr = do("POST", "/admin/calendar/off/"+id+"/delete", url.Values{}, false)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "ok=") {
		t.Errorf("remove: %d %q", rr.Code, rr.Header().Get("Location"))
	}
	if _, ok := f.events[id]; ok {
		t.Error("time off event still in Google after Remove")
	}
	// Removing it again (say, a second tab) is not an error.
	if rr := do("POST", "/admin/calendar/off/"+id+"/delete", url.Values{}, false); !strings.Contains(rr.Header().Get("Location"), "ok=") {
		t.Errorf("second remove: %q", rr.Header().Get("Location"))
	}
}
