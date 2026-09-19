package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/elug3/dupli1/support/pkg/domain"
)

func kst(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.FixedZone("KST", 9*60*60)
	}
	return loc
}

func TestBusinessHoursWindow(t *testing.T) {
	hours := domain.DefaultBusinessHours()
	seoul := kst(t)

	cases := []struct {
		name string
		at   time.Time
		open bool
	}{
		// 2026-09-17 is a Thursday; 09-19 a Saturday; 09-20 a Sunday.
		{"weekday just after opening", time.Date(2026, 9, 17, 10, 0, 0, 0, seoul), true},
		{"weekday midday", time.Date(2026, 9, 17, 14, 30, 0, 0, seoul), true},
		{"weekday one minute before closing", time.Date(2026, 9, 17, 21, 59, 0, 0, seoul), true},
		{"weekday at closing", time.Date(2026, 9, 17, 22, 0, 0, 0, seoul), false},
		{"weekday before opening", time.Date(2026, 9, 17, 9, 59, 0, 0, seoul), false},
		{"weekday middle of the night", time.Date(2026, 9, 17, 3, 0, 0, 0, seoul), false},
		{"saturday midday", time.Date(2026, 9, 19, 14, 0, 0, 0, seoul), false},
		{"sunday midday", time.Date(2026, 9, 20, 14, 0, 0, 0, seoul), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hours.IsOpen(tc.at); got != tc.open {
				t.Fatalf("IsOpen(%s) = %v, want %v", tc.at.Format(time.RFC3339), got, tc.open)
			}
		})
	}
}

func TestHoursAreReadInSeoulNotTheContainerClock(t *testing.T) {
	// ECS runs UTC. 01:30 UTC is 10:30 in Seoul — inside the window — and the
	// same instant expressed in UTC must not read as the middle of the night.
	hours := domain.DefaultBusinessHours()
	utcMorning := time.Date(2026, 9, 17, 1, 30, 0, 0, time.UTC)
	if !hours.IsOpen(utcMorning) {
		t.Fatal("01:30 UTC is 10:30 KST and must count as open")
	}
	utcEvening := time.Date(2026, 9, 17, 14, 0, 0, 0, time.UTC) // 23:00 KST
	if hours.IsOpen(utcEvening) {
		t.Fatal("14:00 UTC is 23:00 KST and must count as closed")
	}
}

func TestNearClosingStopsPromisingSameDay(t *testing.T) {
	// An inquiry at 21:55 must not claim someone is about to answer. Staff may
	// well still be there, so only the promise softens — see the router test
	// for the alert staying loud.
	hours := domain.DefaultBusinessHours()
	seoul := kst(t)

	nearClosing := time.Date(2026, 9, 17, 21, 55, 0, 0, seoul)
	if !hours.IsOpen(nearClosing) {
		t.Fatal("21:55 is still inside the service window")
	}
	if hours.PromisesSameDay(nearClosing) {
		t.Fatal("21:55 must not promise same-day handling")
	}

	comfortablyOpen := time.Date(2026, 9, 17, 21, 30, 0, 0, seoul)
	if !hours.PromisesSameDay(comfortablyOpen) {
		t.Fatal("21:30 leaves time to answer and may promise it")
	}
}

func TestWindowReadsAsKoreanCopy(t *testing.T) {
	if got := domain.DefaultBusinessHours().Window(); got != "평일 10:00~22:00" {
		t.Fatalf("Window() = %q", got)
	}
}

func TestWindowNamesNoDay(t *testing.T) {
	// Without a holiday calendar any named day is a guess.
	window := domain.DefaultBusinessHours().Window()
	for _, day := range []string{"내일", "오늘", "월요일", "화요일", "수요일", "목요일", "금요일"} {
		if strings.Contains(window, day) {
			t.Fatalf("window %q names a day", window)
		}
	}
}

func TestParseWeekdays(t *testing.T) {
	cases := map[string][]time.Weekday{
		"mon-fri":  {time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
		"sat,sun":  {time.Saturday, time.Sunday},
		"":         nil,
		"nonsense": nil,
	}
	for spec, want := range cases {
		got := domain.ParseWeekdays(spec)
		if len(got) != len(want) {
			t.Fatalf("ParseWeekdays(%q) = %v, want %v", spec, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ParseWeekdays(%q) = %v, want %v", spec, got, want)
			}
		}
	}
}

func TestConfiguredWeekendShiftIsHonoured(t *testing.T) {
	// The UTC equivalence the default window happens to have is a convenience,
	// not a licence: a Saturday shift must work without touching the code.
	hours := domain.DefaultBusinessHours()
	hours.Weekdays = domain.ParseWeekdays("sat,sun")
	seoul := kst(t)

	if !hours.IsOpen(time.Date(2026, 9, 19, 14, 0, 0, 0, seoul)) {
		t.Fatal("configured Saturday must count as open")
	}
	if hours.IsOpen(time.Date(2026, 9, 17, 14, 0, 0, 0, seoul)) {
		t.Fatal("Thursday is not in the configured shift")
	}
}
