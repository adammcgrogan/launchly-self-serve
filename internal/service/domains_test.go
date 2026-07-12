package service

import "testing"

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"YourBusiness.com":        "yourbusiness.com",
		"https://example.com/":    "example.com",
		"http://example.com:8080": "example.com",
		"  spaced.com  ":          "spaced.com",
		"example.com/path":        "example.com",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidDomain(t *testing.T) {
	valid := []string{"example.com", "sub.example.com", "my-business.co.uk"}
	invalid := []string{"", "localhost", "not a domain", "-example.com", "example.com-"}

	for _, h := range valid {
		if !validDomain(h) {
			t.Errorf("validDomain(%q) = false, want true", h)
		}
	}
	for _, h := range invalid {
		if validDomain(h) {
			t.Errorf("validDomain(%q) = true, want false", h)
		}
	}
}
