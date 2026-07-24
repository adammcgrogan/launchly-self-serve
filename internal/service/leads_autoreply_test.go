package service

import (
	"testing"
	"time"
)

func TestAutoReplyLimiter(t *testing.T) {
	rl := newAutoReplyLimiter(2, time.Hour)

	if !rl.allow("victim@example.com") {
		t.Fatal("expected 1st send to be allowed")
	}
	if !rl.allow("victim@example.com") {
		t.Fatal("expected 2nd send to be allowed")
	}
	if rl.allow("victim@example.com") {
		t.Fatal("expected 3rd send within the window to be denied")
	}

	if !rl.allow("other@example.com") {
		t.Fatal("expected a different address to have its own window")
	}
}

func TestAutoReplyLimiterWindowResets(t *testing.T) {
	rl := newAutoReplyLimiter(1, time.Hour)

	if !rl.allow("victim@example.com") {
		t.Fatal("expected 1st send to be allowed")
	}
	rl.windows["victim@example.com"].reset = time.Now().Add(-time.Second)

	if !rl.allow("victim@example.com") {
		t.Fatal("expected send to be allowed again once the window has expired")
	}
}
