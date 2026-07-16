package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// csrfTokenTTL bounds how long a leaked token stays valid.
const csrfTokenTTL = 24 * time.Hour

// CSRF derives a token bound to a subject (a user ID or "superadmin"), a
// per-session nonce (from the auth cookie, e.g. the Supabase session ID —
// empty where no such session exists), and an expiry — from the app's
// signing key, so no server-side session storage is required. Binding to
// the session nonce means logging out or starting a new session (a new
// nonce) invalidates previously issued tokens; the expiry bounds how long a
// leaked token stays valid.
type CSRF struct {
	signingKey string
}

func NewCSRF(signingKey string) *CSRF {
	return &CSRF{signingKey: signingKey}
}

func (c *CSRF) Token(subject, sessionNonce string) string {
	exp := time.Now().Add(csrfTokenTTL).Unix()
	return c.sign(subject, sessionNonce, exp)
}

func (c *CSRF) Verify(subject, sessionNonce, token string) bool {
	expPart, _, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expPart, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := c.sign(subject, sessionNonce, exp)
	return hmac.Equal([]byte(token), []byte(expected))
}

func (c *CSRF) sign(subject, sessionNonce string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(c.signingKey))
	mac.Write([]byte("csrf-v1:" + subject + ":" + sessionNonce + ":" + strconv.FormatInt(exp, 10)))
	sum := hex.EncodeToString(mac.Sum(nil))[:32]
	return strconv.FormatInt(exp, 10) + "." + sum
}
