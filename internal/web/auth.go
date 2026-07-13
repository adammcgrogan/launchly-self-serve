package web

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/supabase"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

func (h *Handler) SignupForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "auth:signup", map[string]any{"Next": r.URL.Query().Get("next")})
}

func (h *Handler) SignupSubmit(w http.ResponseWriter, r *http.Request) {
	next := r.FormValue("next")
	if !h.signupLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:signup", map[string]any{
			"Error": "Too many attempts. Please wait a moment and try again.",
			"Email": strings.TrimSpace(strings.ToLower(r.FormValue("email"))), "Next": next,
		})
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
			"Error": "Enter a valid email and a password of at least 8 characters.", "Email": emailAddr, "Next": next,
		})
		return
	}

	sess, err := h.accounts.SignUp(r.Context(), emailAddr, password, next)
	if err != nil {
		errMsg := "Something went wrong creating your account. Please try again."
		if errors.Is(err, supabase.ErrUserAlreadyExists) {
			errMsg = `Looks like you already have an account — <a href="/login" class="underline font-medium">log in instead</a>.`
		}
		h.render.Render(w, "auth:signup", map[string]any{"Error": template.HTML(errMsg), "Email": emailAddr, "Next": next})
		return
	}

	if sess.AccessToken == "" {
		// Email confirmation required before a session exists.
		resendURL := "/resend-verification?email=" + url.QueryEscape(emailAddr)
		if next != "" {
			resendURL += "&next=" + url.QueryEscape(next)
		}
		http.Redirect(w, r, resendURL, http.StatusSeeOther)
		return
	}

	h.auth.SetSessionCookies(w, sess, true)
	if next == "" || !strings.HasPrefix(next, "/dashboard") {
		next = "/dashboard/sites/new"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) LoginForm(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Next": r.URL.Query().Get("next")}
	if r.URL.Query().Get("verified") == "1" {
		data["Info"] = "Email confirmed — log in below."
	}
	h.render.Render(w, "auth:login", data)
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.loginLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:login", map[string]any{
			"Error": "Too many attempts. Please wait a moment and try again.",
			"Email": strings.TrimSpace(strings.ToLower(r.FormValue("email"))), "Next": r.FormValue("next"),
		})
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
	emailAddr := r.URL.Query().Get("email")
	data := map[string]any{"Email": emailAddr, "Next": r.URL.Query().Get("next")}
	if emailAddr != "" {
		data["Info"] = "Account created — check " + emailAddr + " for a confirmation link, or resend it below."
	}
	h.render.Render(w, "auth:resend_verification", data)
}

func (h *Handler) ResendVerificationSubmit(w http.ResponseWriter, r *http.Request) {
	next := r.FormValue("next")
	if !h.resendVerificationLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "auth:resend_verification", map[string]any{
			"Error": "Too many attempts. Please wait a moment and try again.",
			"Email": strings.TrimSpace(strings.ToLower(r.FormValue("email"))), "Next": next,
		})
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
		_ = h.accounts.ResendVerificationEmail(r.Context(), emailAddr, next)
	}
	h.render.Render(w, "auth:resend_verification", map[string]any{
		"Info":  "If an account exists for that email and isn't verified yet, a confirmation link is on its way.",
		"Email": emailAddr, "Next": next,
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
