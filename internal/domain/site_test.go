package domain

import (
	"testing"
	"time"
)

// TestOpenNowEqualTimesNotOvernight covers #249: a BusinessHours row where
// OpensAt == ClosesAt (e.g. a copy-paste mistake, or an attempt at "open 24
// hours") must not be misread as an overnight span (ClosesAt <= OpensAt) that
// leaves the badge showing "Open now" all day and into the next morning.
func TestOpenNowEqualTimesNotOvernight(t *testing.T) {
	var hours []BusinessHours
	for d := time.Sunday; d <= time.Saturday; d++ {
		hours = append(hours, BusinessHours{Weekday: d, OpensAt: "09:00", ClosesAt: "09:00"})
	}
	agg := SiteAggregate{
		Site:          Site{Timezone: "UTC"},
		BusinessHours: hours,
	}

	open, label := agg.OpenNow()
	if open {
		t.Fatalf("expected equal open/close times to read as closed, got open=true label=%q", label)
	}
}

// TestOpenNowGenuineOvernightSpan checks that a real overnight span (closes
// strictly before it opens) still reports open right at OpensAt — guarding
// against an overly strict fix for #249 that would break the overnight
// branch entirely. OpensAt is pinned to the current minute (in UTC) and
// ClosesAt to midnight, so the row is open right now regardless of what time
// the test happens to run, as long as OpensAt isn't itself midnight.
func TestOpenNowGenuineOvernightSpan(t *testing.T) {
	opens := time.Now().UTC().Format("15:04")
	closes := "00:00"
	if opens == closes {
		t.Skip("flaky at the exact midnight minute; negligible in practice")
	}

	var hours []BusinessHours
	for d := time.Sunday; d <= time.Saturday; d++ {
		hours = append(hours, BusinessHours{Weekday: d, OpensAt: opens, ClosesAt: closes})
	}
	agg := SiteAggregate{
		Site:          Site{Timezone: "UTC"},
		BusinessHours: hours,
	}

	open, label := agg.OpenNow()
	if !open {
		t.Fatalf("expected a genuine overnight span to report open right at OpensAt=%s, got open=false label=%q", opens, label)
	}
}
