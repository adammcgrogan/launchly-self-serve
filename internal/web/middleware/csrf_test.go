package middleware

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCSRFVerify(t *testing.T) {
	c := NewCSRF("test-signing-key")
	token := c.Token("user-1", "session-a")

	if !c.Verify("user-1", "session-a", token) {
		t.Fatal("expected valid token to verify")
	}
	if c.Verify("user-2", "session-a", token) {
		t.Fatal("token for one subject should not verify for another")
	}
	if c.Verify("user-1", "session-b", token) {
		t.Fatal("token for one session nonce should not verify for another")
	}
	if c.Verify("user-1", "session-a", token+"tampered") {
		t.Fatal("tampered token should not verify")
	}
	if c.Verify("user-1", "session-a", "") {
		t.Fatal("empty token should not verify")
	}
}

func TestCSRFVerifyExpired(t *testing.T) {
	c := NewCSRF("test-signing-key")
	expired := c.sign("user-1", "session-a", time.Now().Add(-time.Minute).Unix())

	if c.Verify("user-1", "session-a", expired) {
		t.Fatal("expired token should not verify")
	}
}

func TestCSRFDifferentSigningKeysDisagree(t *testing.T) {
	a := NewCSRF("key-a")
	b := NewCSRF("key-b")
	token := a.Token("user-1", "session-a")

	if b.Verify("user-1", "session-a", token) {
		t.Fatal("token signed with one key should not verify under another")
	}
}

func TestCSRFRejectsExtendedExpiry(t *testing.T) {
	c := NewCSRF("test-signing-key")
	token := c.Token("user-1", "session-a")

	exp, rest, ok := strings.Cut(token, ".")
	if !ok {
		t.Fatal("expected token to contain a '.' separator")
	}
	expInt, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		t.Fatalf("expected numeric expiry prefix: %v", err)
	}
	tampered := strconv.FormatInt(expInt+3600, 10) + "." + rest

	if c.Verify("user-1", "session-a", tampered) {
		t.Fatal("token with tampered expiry should not verify")
	}
}
