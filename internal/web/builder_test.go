package web

import "testing"

func TestWizardStepForField(t *testing.T) {
	cases := map[string]int{
		"business name":            1,
		"location":                 1,
		"tagline":                  3,
		"about":                    3,
		"contact phone":            3,
		"contact email":            3,
		"service":                  3,
		"service price":            3,
		"service description":      3,
		"CTA text":                 4,
		"logo URL":                 4,
		"address":                  4,
		"map embed URL":            4,
		"certification":            4,
		"testimonial author name":  4,
		"testimonial quote":        4,
		"gallery image URL":        4,
		"facebook link":            4,
		"unknown field never seen": 1,
		"":                         1,
	}
	for field, want := range cases {
		if got := wizardStepForField(field); got != want {
			t.Errorf("wizardStepForField(%q) = %d, want %d", field, got, want)
		}
	}
}
