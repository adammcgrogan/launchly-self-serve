package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/lib/pq"
)

func TestToSlug(t *testing.T) {
	cases := map[string]string{
		"Joe's Pizza & Pasta":     "joes-pizza-pasta",
		"O'Brien`s Auto Repair":   "obriens-auto-repair",
		"  Leading And Trailing ": "leading-and-trailing",
		"Café Résumé":             "caf-r-sum",
		"日本語 Business":            "business",
		"---":                     "",
		"":                        "",
		"UPPER lower MiXeD":       "upper-lower-mixed",
		"double   spaces":         "double-spaces",
		"a_b.c,d!e?f":             "a-b-c-d-e-f",
	}
	for in, want := range cases {
		if got := toSlug(in); got != want {
			t.Errorf("toSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToSlugLongInput(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := toSlug(long)
	if got != long {
		t.Errorf("toSlug of a long all-lowercase-letter string should be unchanged, got len %d want %d", len(got), len(long))
	}
}

func TestToSlugExported(t *testing.T) {
	// ToSlug is just an exported wrapper around toSlug for external callers.
	if ToSlug("My Business") != toSlug("My Business") {
		t.Fatal("ToSlug should delegate to toSlug")
	}
}

func TestReservedSlugsAreBlocked(t *testing.T) {
	for _, s := range []string{"www", "api", "dashboard", "superadmin", "static"} {
		if !reservedSlugs[s] {
			t.Errorf("expected %q to be a reserved slug", s)
		}
	}
	if reservedSlugs["joes-pizza"] {
		t.Error("expected a normal business slug not to be reserved")
	}
}

func TestIsUniqueSlugViolation(t *testing.T) {
	slugViolation := &pq.Error{Code: "23505", Constraint: "sites_slug_key"}
	otherConstraintViolation := &pq.Error{Code: "23505", Constraint: "sites_owner_user_id_fkey"}
	otherCodeErr := &pq.Error{Code: "23503", Constraint: "sites_slug_key"}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("boom"), false},
		{"slug unique violation", slugViolation, true},
		{"unique violation on a different constraint", otherConstraintViolation, false},
		{"non-unique-violation code on the slug constraint", otherCodeErr, false},
		{"wrapped slug unique violation", fmt.Errorf("insert: %w", slugViolation), true},
	}
	for _, c := range cases {
		if got := isUniqueSlugViolation(c.err); got != c.want {
			t.Errorf("%s: isUniqueSlugViolation() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGuardPublishTransition(t *testing.T) {
	if err := guardPublishTransition(domain.SiteStatusPaused); !errors.Is(err, ErrSitePaused) {
		t.Errorf("expected ErrSitePaused for a paused site, got %v", err)
	}
	for _, status := range []domain.SiteStatus{domain.SiteStatusDraft, domain.SiteStatusLive} {
		if err := guardPublishTransition(status); err != nil {
			t.Errorf("expected no error transitioning from status %q, got %v", status, err)
		}
	}
}
