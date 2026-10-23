package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gutter/db"
)

// Online booking slots. On /book a customer can pick a morning or afternoon
// arrival window instead of asking us to call. A window is offered only when
// nothing overlaps it: no booking in the app, no time off on the app's Google
// calendar (timeoff.go), and no busy time on any of the admin's other Google
// calendars (less the ones excluded in calendar settings).
//
// The check needs the calendar connection. Without it, or when Google can't be
// reached, no windows are offered and the form falls back to "talk to us
// first" — the app never offers a time it couldn't check.
//
// One booking fills a whole window: the visit's start_at is the window's
// opening and its duration the window's length, so the admin calendar and
// Google both hold the full window.

const (
	slotMidday      = "12:00"        // splits a trading day into morning and afternoon
	slotMinWindow   = 3 * time.Hour  // shorter windows (Saturday afternoon) aren't offered
	slotMinNotice   = 12 * time.Hour // a window must open at least this far ahead
	slotHorizonDays = 21             // how far ahead windows are offered
)

// bookingSlot is one arrival window on one day.
type bookingSlot struct {
	Part  string // db.SlotAM | db.SlotPM
	Start time.Time
	End   time.Time
}

// Key identifies the window in the form: "2026-10-20-am".
func (s bookingSlot) Key() string { return s.Start.Format("2006-01-02") + "-" + s.Part }

// PartName is "Morning" or "Afternoon".
func (s bookingSlot) PartName() string {
	if s.Part == db.SlotPM {
		return "Afternoon"
	}
	return "Morning"
}

// Hours renders the window the way the site writes hours: "8am–12pm".
func (s bookingSlot) Hours() string {
	return clock12(s.Start.Format("15:04")) + "–" + clock12(s.End.Format("15:04"))
}

// Label is the full description for emails and the thanks page:
// "Tuesday 20 October, morning (8am–12pm)".
func (s bookingSlot) Label() string {
	return s.Start.Format("Monday 2 January") + ", " + strings.ToLower(s.PartName()) + " (" + s.Hours() + ")"
}

// Short is the compact form for subject lines: "Tue 20 Oct, morning".
func (s bookingSlot) Short() string {
	return s.Start.Format("Mon 2 Jan") + ", " + strings.ToLower(s.PartName())
}

func (s bookingSlot) Minutes() int { return int(s.End.Sub(s.Start).Minutes()) }

// bookingWindow returns the arrival window a booking was booked into, or
// ok=false for an unscheduled booking or one with an exact time.
func bookingWindow(b *db.Booking) (bookingSlot, bool) {
	if b.Slot == "" || b.StartAt.IsZero() {
		return bookingSlot{}, false
	}
	return bookingSlot{Part: b.Slot, Start: b.StartAt.In(db.Melbourne), End: b.EndAt().In(db.Melbourne)}, true
}

// windowsOn returns the windows a day offers, worked out from openHours: the
// trading day split at midday, keeping halves long enough to be worth booking.
func windowsOn(date time.Time) []bookingSlot {
	date = date.In(db.Melbourne)
	code := date.Weekday().String()[:2]
	for _, h := range openHours {
		if h.Code != code {
			continue
		}
		at := func(hhmm string) time.Time {
			t, _ := time.ParseInLocation("2006-01-02 15:04", date.Format("2006-01-02")+" "+hhmm, db.Melbourne)
			return t
		}
		var out []bookingSlot
		add := func(part, from, to string) {
			if from >= to {
				return
			}
			s := bookingSlot{Part: part, Start: at(from), End: at(to)}
			if s.End.Sub(s.Start) >= slotMinWindow {
				out = append(out, s)
			}
		}
		add(db.SlotAM, h.Open, min(h.Close, slotMidday))
		add(db.SlotPM, max(h.Open, slotMidday), h.Close)
		return out
	}
	return nil
}

// candidateSlots lists every window that could be booked now, ignoring the
// calendar: from tomorrow (and at least slotMinNotice away) out to the horizon.
func candidateSlots(now time.Time) []bookingSlot {
	now = now.In(db.Melbourne)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, db.Melbourne)
	var out []bookingSlot
	for d := 1; d <= slotHorizonDays; d++ {
		for _, s := range windowsOn(today.AddDate(0, 0, d)) {
			if s.Start.Sub(now) >= slotMinNotice {
				out = append(out, s)
			}
		}
	}
	return out
}

// slotForKey turns a form value back into a window, accepting only windows
// that are bookable right now.
func slotForKey(key string, now time.Time) (bookingSlot, bool) {
	for _, s := range candidateSlots(now) {
		if s.Key() == key {
			return s, true
		}
	}
	return bookingSlot{}, false
}

// slotRange is the span the availability queries cover: tomorrow to the end
// of the horizon. Day-aligned, so the busy cache key holds for the whole day.
func slotRange(now time.Time) (from, to time.Time) {
	now = now.In(db.Melbourne)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, db.Melbourne)
	return today.AddDate(0, 0, 1), today.AddDate(0, 0, slotHorizonDays+1)
}

// slotTaken reports whether a booking or a busy block overlaps the window.
func slotTaken(s bookingSlot, bs []db.Booking, busy []busyInterval) bool {
	for _, b := range bs {
		if b.StartAt.Before(s.End) && b.EndAt().After(s.Start) {
			return true
		}
	}
	for _, iv := range busy {
		if iv.Start.Before(s.End) && iv.End.After(s.Start) {
			return true
		}
	}
	return false
}

// slotOption is one window as the picker shows it.
type slotOption struct {
	bookingSlot
	Free bool
}

// slotDay is one day in the picker. Only days with a free window are listed.
type slotDay struct {
	Date  time.Time
	Slots []slotOption
}

// availability loads what the windows in [from, to) are checked against.
// Bookings are fetched from a day early so a long visit running into the range
// still counts. Time off, read from the app's own Google calendar alongside
// the busy times, comes back merged into the busy intervals.
func availability(from, to time.Time, fresh bool) ([]db.Booking, []busyInterval, error) {
	if !calendarSyncConnected() {
		return nil, nil, fmt.Errorf("calendar not connected")
	}
	query := busyIntervals
	if fresh {
		query = busyIntervalsFresh
	}
	var (
		off    []timeOff
		offErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() { defer wg.Done(); off, offErr = listTimeOff(from, to, !fresh) }()
	busy, err := query(from, to)
	wg.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("busy times: %w", err)
	}
	if offErr != nil {
		return nil, nil, fmt.Errorf("time off: %w", offErr)
	}
	bs, err := db.ListBookingsBetween(from.Add(-24*time.Hour), to)
	if err != nil {
		return nil, nil, fmt.Errorf("bookings: %w", err)
	}
	// Copy before appending: busy may be the cached slice.
	all := make([]busyInterval, 0, len(busy)+len(off))
	all = append(all, busy...)
	for _, t := range off {
		all = append(all, busyInterval{Start: t.Start, End: t.End})
	}
	return bs, all, nil
}

// openSlots returns the picker's days. ok is false when the calendar can't be
// checked, in which case no window may be offered.
func openSlots(now time.Time) (days []slotDay, ok bool) {
	cands := candidateSlots(now)
	if len(cands) == 0 {
		return nil, true
	}
	from, to := slotRange(now)
	bs, busy, err := availability(from, to, false)
	if err != nil {
		if calendarSyncConnected() {
			logErrorf("slots: %v", err)
		}
		return nil, false
	}
	for _, s := range cands {
		opt := slotOption{bookingSlot: s, Free: !slotTaken(s, bs, busy)}
		day := s.Start.Format("2006-01-02")
		if n := len(days); n > 0 && days[n-1].Date.Format("2006-01-02") == day {
			days[n-1].Slots = append(days[n-1].Slots, opt)
			continue
		}
		days = append(days, slotDay{Date: s.Start, Slots: []slotOption{opt}})
	}
	// Drop fully booked days: a row of greyed-out buttons only adds scrolling.
	kept := days[:0]
	for _, d := range days {
		for _, o := range d.Slots {
			if o.Free {
				kept = append(kept, d)
				break
			}
		}
	}
	return kept, true
}

// slotMu serialises the final check and the booking, so two customers can't
// both take the last free window.
var slotMu sync.Mutex

// slotStillFree re-checks one window against the live calendar, skipping the
// cache. Callers hold slotMu through the check and the booking.
func slotStillFree(s bookingSlot) (bool, error) {
	bs, busy, err := availability(s.Start, s.End, true)
	if err != nil {
		return false, err
	}
	return !slotTaken(s, bs, busy), nil
}

// fmtVisit renders when a visit is: "Tue 20 Oct, morning" for an arrival
// window, else the exact time ("Tue 20 Oct, 9:30 am"). "" when unscheduled.
func fmtVisit(b *db.Booking) string {
	if s, ok := bookingWindow(b); ok {
		return s.Short()
	}
	return fmtWhen(b.StartAt)
}
