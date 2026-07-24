package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// signTestAccessToken builds a minimal Supabase-shaped access token JWT
// (HS256, matching supabase.VerifyAccessToken's expectations) so tests can
// exercise auth.RequireUser without a real Supabase round trip.
func signTestAccessToken(t *testing.T, secret string, userID uuid.UUID, sessionID string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":        userID.String(),
		"session_id": sessionID,
		"exp":        time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

// TestCSRFTokenRefresh covers #194: a dashboard tab left open past the CSRF
// token's 24h TTL still has a valid session, so /dashboard/csrf-token should
// transparently mint a fresh token for that session — but only for an
// authenticated caller, never for an anonymous one.
func TestCSRFTokenRefresh(t *testing.T) {
	const secret = "test-jwt-secret"
	h := &Handler{
		auth: middleware.NewAuth(secret, nil, false),
		csrf: middleware.NewCSRF("test-signing-key"),
	}
	route := h.auth.RequireUser(h.CSRFTokenRefresh)

	t.Run("authenticated request gets a fresh valid token", func(t *testing.T) {
		userID := uuid.New()
		access := signTestAccessToken(t, secret, userID, "session-a")

		req := httptest.NewRequest(http.MethodGet, "/dashboard/csrf-token", nil)
		req.AddCookie(&http.Cookie{Name: "sb_access_token", Value: access})
		w := httptest.NewRecorder()

		route(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
		}
		var body struct {
			CSRFToken string `json:"csrf_token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.CSRFToken == "" {
			t.Fatal("expected a non-empty csrf_token")
		}
		if !h.csrf.Verify(userID.String(), "session-a", body.CSRFToken) {
			t.Error("returned token does not verify for the requesting user's session")
		}
	})

	t.Run("unauthenticated request is rejected, not issued a token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/csrf-token", nil)
		w := httptest.NewRecorder()

		route(w, req)

		if w.Code == http.StatusOK {
			t.Fatalf("expected an anonymous request to be rejected, got 200 with body %q", w.Body.String())
		}
	})
}
