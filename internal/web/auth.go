package web

import (
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

func (h *Handler) SignupForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:signup", map[string]any{"Next": r.URL.Query().Get("next")})
}

func (h *Handler) SignupSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.signupLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:signup", map[string]any{"Error": "Too many attempts. Please wait a moment and try again."})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	emailAddr := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	if emailAddr == "" || len(password) < 8 {
		h.render.Render(w, "auth:signup", map[string]any{
			"Error": "Enter a valid email and a password of at least 8 characters.", "Email": emailAddr,
		})
		return
	}

	sess, err := h.accounts.SignUp(r.Context(), emailAddr, password)
	if err != nil {
		h.render.Render(w, "auth:signup", map[string]any{"Error": err.Error(), "Email": emailAddr})
		return
	}

	if sess.AccessToken == "" {
		// Email confirmation required before a session exists.
		h.render.Render(w, "auth:login", map[string]any{
			"Info": "Account created — check your email to confirm it, then log in.",
		})
		return
	}

	h.auth.SetSessionCookies(w, sess, true)
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/dashboard") {
		next = "/dashboard/sites/new"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) LoginForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:login", map[string]any{"Next": r.URL.Query().Get("next")})
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.loginLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:login", map[string]any{"Error": "Too many attempts. Please wait a moment and try again."})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	emailAddr := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	next := r.FormValue("next")
	rememberMe := r.FormValue("remember_me") != ""

	sess, err := h.accounts.Login(r.Context(), emailAddr, password)
	if err != nil {
		h.render.Render(w, "auth:login", map[string]any{"Error": "Incorrect email or password.", "Email": emailAddr, "Next": next})
		return
	}

	h.auth.SetSessionCookies(w, sess, rememberMe)
	if next == "" || !strings.HasPrefix(next, "/dashboard") {
		next = "/dashboard"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if token := h.auth.AccessToken(r); token != "" {
		_ = h.accounts.Logout(r.Context(), token)
	}
	h.auth.ClearSessionCookies(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) ForgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:forgot_password", map[string]any{})
}

func (h *Handler) ForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.loginLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:forgot_password", map[string]any{"Error": "Too many attempts. Please wait a moment and try again."})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	emailAddr := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if emailAddr != "" {
		// Errors are intentionally swallowed: don't reveal whether an
		// account exists for this address.
		_ = h.accounts.RequestPasswordReset(r.Context(), emailAddr)
	}
	h.render.Render(w, "auth:forgot_password", map[string]any{
		"Info": "If an account exists for that email, a reset link is on its way.",
	})
}

// ResetPasswordForm renders a page that reads the recovery access token out
// of the URL fragment (Supabase puts it after '#', so it's only visible
// client-side) and submits it alongside the new password.
func (h *Handler) ResetPasswordForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:reset_password", map[string]any{})
}

func (h *Handler) ResendVerificationForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:resend_verification", map[string]any{})
}

func (h *Handler) ResendVerificationSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.resendVerificationLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:resend_verification", map[string]any{"Error": "Too many attempts. Please wait a moment and try again."})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	emailAddr := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if emailAddr != "" && h.resendVerificationLimiter.Allow("email:"+emailAddr) {
		// Errors are intentionally swallowed: don't reveal whether an
		// account exists or is already verified.
		_ = h.accounts.ResendVerificationEmail(r.Context(), emailAddr)
	}
	h.render.Render(w, "auth:resend_verification", map[string]any{
		"Info": "If an account exists for that email and isn't verified yet, a confirmation link is on its way.",
	})
}

func (h *Handler) ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	accessToken := r.FormValue("access_token")
	password := r.FormValue("password")
	if accessToken == "" || len(password) < 8 {
		h.render.Render(w, "auth:reset_password", map[string]any{"Error": "Enter a password of at least 8 characters."})
		return
	}
	if err := h.accounts.UpdatePassword(r.Context(), accessToken, password); err != nil {
		h.render.Render(w, "auth:reset_password", map[string]any{"Error": "That reset link has expired. Request a new one."})
		return
	}
	middleware.SetFlash(w, "Password updated — log in with your new password.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
