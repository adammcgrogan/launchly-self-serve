package web

import (
	"testing"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// TestDayKeyUsesSiteTimezone guards against the #192 regression: GetSiteStats
// buckets days by the site's own timezone, so a DayCount.Day value (which
// Postgres/json round-tripping hands back as the correct instant, but
// labeled in the Go time.Time's UTC location) must be keyed by converting
// into the site's timezone — not by calling .UTC() on it, which shifts the
// calendar date by a day for any timezone ahead of UTC.
func TestDayKeyUsesSiteTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// Local midnight, 20 July 2026, in Sydney (AEST, UTC+10) is 19 July
	// 14:00 UTC — a different calendar date in UTC.
	local := time.Date(2026, 7, 20, 0, 0, 0, 0, loc)
	decoded := local.UTC() // simulates the value as decoded from Postgres/JSON

	if got, want := decoded.Format("2006-01-02"), "2026-07-19"; got != want {
		t.Fatalf("test setup invalid: decoded instant should format as %s in UTC, got %s", want, got)
	}

	if key := dayKey(decoded, loc); key != "2026-07-20" {
		t.Errorf("dayKey(decoded, Sydney) = %q, want 2026-07-20", key)
	}
}

// TestLastNDayPointsNonUTCTimezone checks that a view recorded for "today" in
// a non-UTC site timezone lands in today's bucket, not yesterday's — the
// symptom reported in #192 (bars dropped or shifted a day for non-UTC
// sites).
func TestLastNDayPointsNonUTCTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	now := time.Now().In(loc)
	todayLocalMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// Simulate GetSiteStats' returned DayCount.Day: the correct instant,
	// decoded with a UTC-labeled time.Time.
	decodedDay := todayLocalMidnight.UTC()

	viewsByDay := []domain.DayCount{{Day: decodedDay, Count: 5}}
	points := lastNDayPoints(viewsByDay, 1, loc)

	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].Count != 5 {
		t.Errorf("today's bucket count = %d, want 5 (bug: view landed in the wrong day's bucket)", points[0].Count)
	}
}

// TestCSVSafe guards the CSV-injection defense applied to visitor-controlled
// lead fields (name, email, phone, message, ...) on export — see #251. A
// leading =, +, -, @, tab, or CR would otherwise make Excel/Sheets evaluate
// the cell as a formula when the owner opens their leads export.
func TestCSVSafe(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"formula injection with SUM", "=SUM(A1:A9)", "'=SUM(A1:A9)"},
		{"plus-prefixed formula", "+1+1", "'+1+1"},
		{"minus-prefixed formula", "-2+3", "'-2+3"},
		{"at-prefixed formula", "@SUM(1,2)", "'@SUM(1,2)"},
		{"tab-prefixed", "\tmalicious", "'\tmalicious"},
		{"CR-prefixed", "\rmalicious", "'\rmalicious"},
		{"ordinary name untouched", "Jane Doe", "Jane Doe"},
		{"ordinary email untouched", "jane@example.com", "jane@example.com"},
		{"empty string untouched", "", ""},
		{"dangerous char mid-string is fine", "call me @ noon", "call me @ noon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := csvSafe(tt.in); got != tt.want {
				t.Errorf("csvSafe(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
