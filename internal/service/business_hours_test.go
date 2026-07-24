package service

import (
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// TestValidateBusinessHoursRejectsEqualTimes covers #249: OpensAt == ClosesAt
// should be rejected at save time rather than silently stored and later
// misread by domain.SiteAggregate.OpenNow as an overnight span.
func TestValidateBusinessHoursRejectsEqualTimes(t *testing.T) {
	hours := []domain.BusinessHours{
		{Weekday: 1, OpensAt: "09:00", ClosesAt: "09:00"},
	}
	if err := validateBusinessHours(hours); err == nil {
		t.Fatal("expected an error when OpensAt equals ClosesAt")
	}
}

func TestValidateBusinessHoursAllowsGenuineHours(t *testing.T) {
	hours := []domain.BusinessHours{
		{Weekday: 1, OpensAt: "09:00", ClosesAt: "17:00"},
		{Weekday: 2, OpensAt: "18:00", ClosesAt: "02:00"}, // genuine overnight span
		{Weekday: 3, Closed: true},
	}
	if err := validateBusinessHours(hours); err != nil {
		t.Fatalf("expected valid hours to pass, got %v", err)
	}
}
