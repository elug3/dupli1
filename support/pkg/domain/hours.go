package domain

import (
	"strings"
	"time"
)

// BusinessHours is when staff answer: weekdays 10:00–22:00 KST, closed at
// weekends and on public holidays.
//
// Holidays are deliberately not modelled. Managers simply do not answer on
// them, and a calendar nobody refills each January is worse than none because
// it is silently wrong — Seollal and Chuseok are lunar and substitute holidays
// are declared per year, so no rule derives them. The cost is paid in the copy:
// the bot never names a day it will reply, only the window. See
// docs/support-telegram-bot.md.
type BusinessHours struct {
	Location  *time.Location
	OpenHour  int
	OpenMin   int
	CloseHour int
	CloseMin  int
	// Weekdays staff work. Empty means Monday–Friday.
	Weekdays []time.Weekday
	// WindDown is how long before closing an inquiry stops being promised
	// same-day handling. Staff may well still be there, so the alert stays
	// loud; only the promise softens.
	WindDown time.Duration
}

// DefaultBusinessHours is the launch configuration.
func DefaultBusinessHours() BusinessHours {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		// A container without tzdata would otherwise silently fall back to UTC
		// and answer nine hours out. Korea has no DST, so a fixed offset is
		// exact, unlike guessing.
		seoul = time.FixedZone("KST", 9*60*60)
	}
	return BusinessHours{
		Location:  seoul,
		OpenHour:  10,
		CloseHour: 22,
		WindDown:  15 * time.Minute,
	}
}

// IsOpen reports whether staff are working at t.
func (h BusinessHours) IsOpen(t time.Time) bool {
	return h.openAt(t, 0)
}

// PromisesSameDay reports whether an inquiry opened at t can honestly be told
// someone is about to look at it.
func (h BusinessHours) PromisesSameDay(t time.Time) bool {
	return h.openAt(t, h.WindDown)
}

func (h BusinessHours) openAt(t time.Time, windDown time.Duration) bool {
	local := t.In(h.location())
	if !h.worksOn(local.Weekday()) {
		return false
	}
	open := time.Date(local.Year(), local.Month(), local.Day(), h.OpenHour, h.OpenMin, 0, 0, h.location())
	close := time.Date(local.Year(), local.Month(), local.Day(), h.CloseHour, h.CloseMin, 0, 0, h.location()).Add(-windDown)
	return !local.Before(open) && local.Before(close)
}

func (h BusinessHours) worksOn(day time.Weekday) bool {
	if len(h.Weekdays) == 0 {
		return day != time.Saturday && day != time.Sunday
	}
	for _, workday := range h.Weekdays {
		if workday == day {
			return true
		}
	}
	return false
}

func (h BusinessHours) location() *time.Location {
	if h.Location != nil {
		return h.Location
	}
	return time.UTC
}

// Window renders the service hours for a shopper, e.g. "평일 10:00~22:00".
func (h BusinessHours) Window() string {
	return "평일 " + clock(h.OpenHour, h.OpenMin) + "~" + clock(h.CloseHour, h.CloseMin)
}

func clock(hour, min int) string {
	return twoDigits(hour) + ":" + twoDigits(min)
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// ParseWeekdays reads a "mon-fri" or "mon,wed,fri" specification.
// An unreadable value returns nil, which means the Monday–Friday default.
func ParseWeekdays(spec string) []time.Weekday {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" {
		return nil
	}
	names := map[string]time.Weekday{
		"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
		"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday,
		"sat": time.Saturday,
	}
	if from, to, ranged := strings.Cut(spec, "-"); ranged {
		start, okStart := names[strings.TrimSpace(from)]
		end, okEnd := names[strings.TrimSpace(to)]
		if !okStart || !okEnd {
			return nil
		}
		var days []time.Weekday
		for day := start; ; day = (day + 1) % 7 {
			days = append(days, day)
			if day == end {
				break
			}
			if len(days) > 7 {
				return nil
			}
		}
		return days
	}
	var days []time.Weekday
	for _, name := range strings.Split(spec, ",") {
		day, ok := names[strings.TrimSpace(name)]
		if !ok {
			return nil
		}
		days = append(days, day)
	}
	return days
}
