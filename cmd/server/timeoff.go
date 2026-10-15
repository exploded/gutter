package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gutter/db"
)

// Time off: spans the admin blocks out, either from /admin/calendar or by
// adding an event to the app's Google calendar. Google is the only record:
// the form writes the event there and every view reads it back, so moving or
// deleting it in Google on the phone just works. A span hides every online
// booking window it touches (availability in slots.go) and is drawn on the
// admin calendar.
//
// Every event on the app's calendar that isn't a booking counts, unless it is
// marked "free" in Google. Bookings stay one-way: moving a booking in Google
// changes nothing, because a reschedule has to email the customer.

const (
	timeOffMaxDays  = 92  // longest span one entry can block, to catch typos
	timeOffNoteMax  = 200 // characters
	timeOffListDays = 180 // how far ahead the "Coming up" list looks
	timeOffTitle    = "Time off"
)

// timeOffProp marks events the form created. Reading doesn't depend on it
// (events added in Google count too); it only tells them apart in Google.
const timeOffProp = "gutter_time_off"

// timeOff is one span read back from Google.
type timeOff struct {
	EventID string
	Start   time.Time
	End     time.Time // exclusive
	Note    string    // the event title, less the "Time off: " the form adds
}

// AllDay reports whether the span is whole days: midnight to midnight.
func (t timeOff) AllDay() bool {
	s, e := t.Start.In(db.Melbourne), t.End.In(db.Melbourne)
	return s.Hour() == 0 && s.Minute() == 0 && e.Hour() == 0 && e.Minute() == 0
}

// timeOffCache memoises the events read per range, like busyCache. Adding or
// removing time off in the app clears it; a change made in Google shows up
// once the entry expires.
var timeOffCache = &rangeCache[[]timeOff]{ttl: 2 * time.Minute}

// listTimeOff reads the time off overlapping [from, to) from the app's Google
// calendar. It returns (nil, nil) when sync isn't connected, and never blocks
// longer than busyTimeout. cached=false skips the cache, for the last check
// before a customer's window is booked.
func listTimeOff(from, to time.Time, cached bool) ([]timeOff, error) {
	g, err := db.GetGoogleCalendar()
	if err != nil {
		return nil, err
	}
	if !g.Connected() {
		return nil, nil
	}
	key := from.Format(time.RFC3339) + "/" + to.Format(time.RFC3339)
	if vals, ok := timeOffCache.get(key, time.Now()); ok && cached {
		return vals, nil
	}
	api, err := gcalClientFor(g)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), busyTimeout)
	defer cancel()
	evs, err := api.ListEvents(ctx, g.CalendarID, from, to)
	if err != nil {
		return nil, err
	}
	out := timeOffFromEvents(evs)
	timeOffCache.put(key, time.Now(), out)
	return out, nil
}

// timeOffFromEvents keeps the events that are time off: not a booking, not
// marked free, with readable times.
func timeOffFromEvents(evs []gcalEvent) []timeOff {
	var out []timeOff
	for i := range evs {
		ev := &evs[i]
		if isBookingEvent(ev) || ev.Transparency == "transparent" {
			continue
		}
		start, end, ok := eventSpan(ev)
		if !ok {
			continue
		}
		out = append(out, timeOff{EventID: ev.ID, Start: start, End: end, Note: timeOffNote(ev.Summary)})
	}
	return out
}

// isBookingEvent reports whether an event is one the booking sync pushed.
func isBookingEvent(ev *gcalEvent) bool {
	return ev.ExtendedProperties != nil && ev.ExtendedProperties.Private[bookingIDProp] != ""
}

// timeOffNote turns an event title into the note shown in the app: the form's
// "Time off: Dentist" becomes "Dentist", and a title typed in Google is kept.
func timeOffNote(summary string) string {
	s := strings.TrimSpace(summary)
	if rest, ok := strings.CutPrefix(s, timeOffTitle+":"); ok {
		return strings.TrimSpace(rest)
	}
	if s == timeOffTitle {
		return ""
	}
	return s
}

// eventForTimeOff builds the Google event for a new span: an all-day event
// for whole days, a timed one otherwise.
func eventForTimeOff(t timeOff) *gcalEvent {
	title := timeOffTitle
	if t.Note != "" {
		title += ": " + t.Note
	}
	link := site.BaseURL + "/admin/calendar?week=" + startOfWeek(t.Start).Format("2006-01-02")
	ev := &gcalEvent{
		Summary: title,
		Description: "Blocked out, so no online bookings land in this time. " +
			"Move or delete it here or in the app: the website follows either way.\n\n" + link,
		Reminders:          &gcalReminders{UseDefault: false},
		ExtendedProperties: &gcalExtProps{Private: map[string]string{timeOffProp: "1"}},
		Source:             &gcalEventSource{Title: businessName, URL: link},
	}
	if t.AllDay() {
		ev.Start = &gcalEventTime{Date: db.FormatDate(t.Start)}
		ev.End = &gcalEventTime{Date: db.FormatDate(t.End)}
		return ev
	}
	const layout = "2006-01-02T15:04:05"
	ev.Start = &gcalEventTime{DateTime: t.Start.In(db.Melbourne).Format(layout), TimeZone: db.Melbourne.String()}
	ev.End = &gcalEventTime{DateTime: t.End.In(db.Melbourne).Format(layout), TimeZone: db.Melbourne.String()}
	return ev
}

// ── Admin form ──

// timeOffForm is the "Block out time" form's values, kept on a validation
// error so nothing has to be typed twice.
type timeOffForm struct {
	Date  string // 'YYYY-MM-DD'
	Until string // last day, '' = same day
	From  string // 'HH:MM', '' = from the start of the day
	To    string // 'HH:MM', '' = to the end of the last day
	Note  string
	Err   string
	Week  string // week param to return to
}

// timeOffFromForm reads the form fields.
func timeOffFromForm(r *http.Request) timeOffForm {
	return timeOffForm{
		Date:  strings.TrimSpace(r.FormValue("date")),
		Until: strings.TrimSpace(r.FormValue("until")),
		From:  strings.TrimSpace(r.FormValue("from")),
		To:    strings.TrimSpace(r.FormValue("to")),
		Note:  strings.TrimSpace(r.FormValue("note")),
		Week:  strings.TrimSpace(r.FormValue("week")),
	}
}

// span turns the form into [start, end). The span runs continuously from the
// first day's start time to the last day's end time; a blank start time means
// midnight and a blank end time means the end of the last day.
func (f timeOffForm) span(now time.Time) (start, end time.Time, msg string) {
	day := func(s string) (time.Time, bool) {
		t, err := time.ParseInLocation("2006-01-02", s, db.Melbourne)
		return t, err == nil
	}
	clock := func(d time.Time, hhmm string) (time.Time, bool) {
		t, err := time.ParseInLocation("2006-01-02 15:04", d.Format("2006-01-02")+" "+hhmm, db.Melbourne)
		return t, err == nil
	}
	first, ok := day(f.Date)
	if !ok {
		return start, end, "Choose the day to block out."
	}
	last := first
	if f.Until != "" {
		if last, ok = day(f.Until); !ok {
			return start, end, "The last day isn't a valid date."
		}
		if last.Before(first) {
			return start, end, "The last day is before the first."
		}
	}
	start = first
	if f.From != "" {
		if start, ok = clock(first, f.From); !ok {
			return start, end, "The start time isn't a valid time."
		}
	}
	end = last.AddDate(0, 0, 1)
	if f.To != "" {
		if end, ok = clock(last, f.To); !ok {
			return start, end, "The end time isn't a valid time."
		}
	}
	switch {
	case !end.After(start):
		return start, end, "The end has to be after the start."
	case end.Sub(start) > timeOffMaxDays*24*time.Hour:
		return start, end, fmt.Sprintf("That's more than %d days. Block out a long break in parts.", timeOffMaxDays)
	case !end.After(now):
		return start, end, "That time has already passed."
	}
	return start, end, ""
}

// fmtTimeOff renders a span compactly: "Tue 20 Oct, all day",
// "Tue 20 – Fri 23 Oct", "Tue 20 Oct, 9:00 am – 1:00 pm" or
// "Tue 20 Oct, 2:00 pm – Wed 21 Oct, 10:00 am".
func fmtTimeOff(t timeOff) string {
	s, e := t.Start.In(db.Melbourne), t.End.In(db.Melbourne)
	if t.AllDay() {
		last := e.AddDate(0, 0, -1)
		if db.FormatDate(last) == db.FormatDate(s) {
			return s.Format("Mon 2 Jan") + ", all day"
		}
		if s.Month() == last.Month() && s.Year() == last.Year() {
			return s.Format("Mon 2") + " – " + last.Format("Mon 2 Jan")
		}
		return s.Format("Mon 2 Jan") + " – " + last.Format("Mon 2 Jan")
	}
	if db.FormatDate(s) == db.FormatDate(e) {
		return s.Format("Mon 2 Jan") + ", " + fmtClock(s) + " – " + fmtClock(e)
	}
	return s.Format("Mon 2 Jan") + ", " + fmtClock(s) + " – " + e.Format("Mon 2 Jan") + ", " + fmtClock(e)
}

// calendarURL is the admin calendar for the week holding t, with a flash.
func calendarURL(week time.Time, key, msg string) string {
	u := "/admin/calendar?week=" + startOfWeek(week).Format("2006-01-02")
	if msg != "" {
		u += "&" + key + "=" + url.QueryEscape(msg)
	}
	return u + "#time-off"
}

// handleAdminTimeOffCreate blocks out a span by adding it to Google. On a bad
// form htmx gets the form back with the message and the typed values; on
// success the whole calendar reloads on the span's week, since the grid, the
// list and the flash all change.
func handleAdminTimeOffCreate(w http.ResponseWriter, r *http.Request) {
	f := timeOffFromForm(r)
	if rs := []rune(f.Note); len(rs) > timeOffNoteMax {
		f.Note = string(rs[:timeOffNoteMax])
	}
	start, end, msg := f.span(time.Now())
	if msg != "" {
		timeOffFormError(w, r, f, msg)
		return
	}
	g, api, ok := calendarTarget()
	if !ok {
		timeOffFormError(w, r, f, "Time off lives in Google Calendar: connect it in Calendar sync first.")
		return
	}
	t := timeOff{Start: start, End: end, Note: f.Note}
	ctx, cancel := context.WithTimeout(r.Context(), gcalTimeout)
	defer cancel()
	if _, err := api.InsertEvent(ctx, g.CalendarID, eventForTimeOff(t)); err != nil {
		logGCal("add time off", err)
		timeOffFormError(w, r, f, "Google Calendar didn't take it: "+err.Error())
		return
	}
	timeOffCache.clear()

	done := "Blocked out " + fmtTimeOff(t) + "."
	if clash := timeOffClashes(start, end); clash != "" {
		done += " It overlaps " + clash + ", which is still booked: reschedule or cancel it from the booking."
	}
	dest := calendarURL(start, "ok", done)
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", dest)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// timeOffFormError answers a rejected form: the form fragment under 422 for
// htmx, else a redirect with the message (the typed values are lost then).
func timeOffFormError(w http.ResponseWriter, r *http.Request, f timeOffForm, msg string) {
	if !isHTMX(r) {
		week := db.Today()
		if t, err := time.ParseInLocation("2006-01-02", f.Week, db.Melbourne); err == nil {
			week = t
		}
		http.Redirect(w, r, calendarURL(week, "err", msg), http.StatusSeeOther)
		return
	}
	f.Err = msg
	renderFragment(w, r, "admin-calendar", "time-off-form", http.StatusUnprocessableEntity, calendarData{Off: f, BusyOn: true})
}

// timeOffClashes names the bookings a new span overlaps, "" if none.
func timeOffClashes(start, end time.Time) string {
	// A visit starting up to a day earlier can still run into the span.
	bs, err := db.ListBookingsBetween(start.Add(-24*time.Hour), end)
	if err != nil {
		log.Printf("time off: clashes: %v", err)
		return ""
	}
	var names []string
	for _, b := range bs {
		if b.StartAt.Before(end) && b.EndAt().After(start) {
			names = append(names, "#"+strconv.FormatInt(b.ID, 10)+" "+b.Name+" ("+fmtVisit(&b)+")")
		}
	}
	return strings.Join(names, ", ")
}

// handleAdminTimeOffDelete removes a span's event from Google. It refuses
// booking events: those go through the booking page so the customer hears.
func handleAdminTimeOffDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, api, ok := calendarTarget()
	if !ok {
		http.Redirect(w, r, calendarURL(db.Today(), "err", "Google Calendar isn't connected."), http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), gcalTimeout)
	defer cancel()
	ev, err := api.GetEvent(ctx, g.CalendarID, id)
	if errors.Is(err, errNotFound) {
		timeOffCache.clear()
		http.Redirect(w, r, calendarURL(db.Today(), "ok", "That time off was already gone."), http.StatusSeeOther)
		return
	}
	if err != nil {
		logGCal("load time off", err)
		http.Redirect(w, r, calendarURL(db.Today(), "err", "Couldn't reach Google Calendar: "+err.Error()), http.StatusSeeOther)
		return
	}
	offs := timeOffFromEvents([]gcalEvent{*ev})
	if len(offs) == 0 {
		http.Redirect(w, r, calendarURL(db.Today(), "err", "That event isn't time off. Change bookings from the booking page."), http.StatusSeeOther)
		return
	}
	t := offs[0]
	if err := api.DeleteEvent(ctx, g.CalendarID, id); err != nil && !errors.Is(err, errNotFound) {
		logGCal("remove time off", err)
		http.Redirect(w, r, calendarURL(t.Start, "err", "Google Calendar didn't remove it: "+err.Error()), http.StatusSeeOther)
		return
	}
	timeOffCache.clear()
	http.Redirect(w, r, calendarURL(t.Start, "ok", "Removed time off "+fmtTimeOff(t)+"."), http.StatusSeeOther)
}
