package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// CSRF derives a per-subject token (subject being a user ID or "superadmin")
// from the app's signing key — no server-side session storage required.
type CSRF struct {
	signingKey string
}

func NewCSRF(signingKey string) *CSRF {
	return &CSRF{signingKey: signingKey}
}

func (c *CSRF) Token(subject string) string {
	mac := hmac.New(sha256.New, []byte(c.signingKey))
	mac.Write([]byte("csrf-v1:" + subject))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func (c *CSRF) Verify(subject, token string) bool {
	expected := c.Token(subject)
	return hmac.Equal([]byte(token), []byte(expected))
}
