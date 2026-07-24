package web

import "testing"

func TestIsCommonPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"common lowercase", "password1", true},
		{"common uppercase mixed case", "Password1", true},
		{"common mixed case exact", "PaSSw0rd", true},
		{"common numeric", "12345678", true},
		{"strong uncommon password", "Tr0ub4dor&Zeb3llina!", false},
		{"uncommon passphrase", "correct-horse-battery-staple-42", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCommonPassword(tt.password); got != tt.want {
				t.Errorf("isCommonPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}
