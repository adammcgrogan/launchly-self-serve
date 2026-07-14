// Package supabase is a thin REST client for the Supabase Auth (GoTrue) API,
// plus local verification of the JWTs it issues. Our Go server acts as a
// backend-for-frontend: it calls this API on signup/login, stores the
// returned tokens in httpOnly cookies, and verifies them locally on
// subsequent requests (see jwt.go) rather than round-tripping to Supabase.
package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Client talks to the Supabase Auth REST API for a single project.
type Client struct {
	baseURL        string // e.g. https://xyzcompany.supabase.co
	anonKey        string
	serviceRoleKey string // only needed for admin endpoints, e.g. DeleteUser
	http           *http.Client
}

func NewClient(baseURL, anonKey, serviceRoleKey string) *Client {
	return &Client{
		baseURL:        baseURL,
		anonKey:        anonKey,
		serviceRoleKey: serviceRoleKey,
		http:           &http.Client{Timeout: 10 * time.Second},
	}
}

// Session is the token pair + identity returned by a successful signup/login.
type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	UserID       uuid.UUID
	Email        string
}

type gotrueUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type gotrueTokenResponse struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	ExpiresIn    int        `json:"expires_in"`
	User         gotrueUser `json:"user"`
}

// gotrueError models the two error response shapes GoTrue has used across
// versions; whichever fields are present, one of these will be non-empty.
type gotrueError struct {
	Msg              string `json:"msg"`
	ErrorDescription string `json:"error_description"`
	ErrorCode        string `json:"error_code"`
	Message          string `json:"message"`
}

func (e gotrueError) String() string {
	for _, s := range []string{e.Msg, e.ErrorDescription, e.Message, e.ErrorCode} {
		if s != "" {
			return s
		}
	}
	return "unknown auth error"
}

func (c *Client) do(ctx context.Context, method, path string, body any, authHeader string) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.anonKey)
	if authHeader != "" {
		req.Header.Set("Authorization", "Bearer "+authHeader)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.anonKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}

func toSession(t gotrueTokenResponse) (*Session, error) {
	id, err := uuid.Parse(t.User.ID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	return &Session{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		ExpiresIn:    t.ExpiresIn,
		UserID:       id,
		Email:        t.User.Email,
	}, nil
}

// ErrUserAlreadyExists is returned by SignUp when the email is already
// registered, so callers can show a friendly "log in instead" message
// rather than a raw GoTrue error string.
var ErrUserAlreadyExists = errors.New("user already registered")

// ErrEmailNotConfirmed is returned by SignInWithPassword when the account's
// email hasn't been confirmed yet (only possible if Supabase's "Confirm
// email" project setting is enabled) — callers should point the user at
// resending the confirmation email rather than showing a generic
// wrong-password error.
var ErrEmailNotConfirmed = errors.New("email not confirmed")

// SignUp creates a new Supabase auth user and, if email confirmation is
// disabled on the project, returns an active session. If confirmation is
// required, AccessToken will be empty — the caller should treat this as
// "account created, check your email" rather than an error. redirectTo
// tells Supabase where the confirmation link should land — without it,
// Supabase falls back to its project-level default, which may not be our
// login page.
func (c *Client) SignUp(ctx context.Context, email, password, redirectTo string) (*Session, error) {
	respBody, status, err := c.do(ctx, http.MethodPost, "/auth/v1/signup", map[string]string{
		"email": email, "password": password, "redirect_to": redirectTo,
	}, "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		msg := strings.ToLower(gErr.String())
		if gErr.ErrorCode == "user_already_exists" || gErr.ErrorCode == "email_exists" ||
			strings.Contains(msg, "already registered") || strings.Contains(msg, "already exists") {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("supabase signup failed: %s", gErr.String())
	}
	var t gotrueTokenResponse
	if err := json.Unmarshal(respBody, &t); err != nil {
		return nil, fmt.Errorf("decode signup response: %w", err)
	}
	if t.User.ID == "" {
		return nil, fmt.Errorf("supabase signup returned no user")
	}
	if t.AccessToken == "" {
		// Email confirmation required — no session yet.
		id, err := uuid.Parse(t.User.ID)
		if err != nil {
			return nil, fmt.Errorf("parse user id: %w", err)
		}
		return &Session{UserID: id, Email: t.User.Email}, nil
	}
	return toSession(t)
}

// SignInWithPassword logs in an existing user with email + password.
func (c *Client) SignInWithPassword(ctx context.Context, email, password string) (*Session, error) {
	respBody, status, err := c.do(ctx, http.MethodPost, "/auth/v1/token?grant_type=password", map[string]string{
		"email": email, "password": password,
	}, "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		if gErr.ErrorCode == "email_not_confirmed" || strings.Contains(strings.ToLower(gErr.String()), "not confirmed") {
			return nil, ErrEmailNotConfirmed
		}
		return nil, fmt.Errorf("supabase login failed: %s", gErr.String())
	}
	var t gotrueTokenResponse
	if err := json.Unmarshal(respBody, &t); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	return toSession(t)
}

// RefreshSession exchanges a refresh token for a new access/refresh token pair.
func (c *Client) RefreshSession(ctx context.Context, refreshToken string) (*Session, error) {
	respBody, status, err := c.do(ctx, http.MethodPost, "/auth/v1/token?grant_type=refresh_token", map[string]string{
		"refresh_token": refreshToken,
	}, "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		return nil, fmt.Errorf("supabase refresh failed: %s", gErr.String())
	}
	var t gotrueTokenResponse
	if err := json.Unmarshal(respBody, &t); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}
	return toSession(t)
}

// SignOut invalidates the given access token's session on Supabase's side.
func (c *Client) SignOut(ctx context.Context, accessToken string) error {
	_, status, err := c.do(ctx, http.MethodPost, "/auth/v1/logout", nil, accessToken)
	if err != nil {
		return err
	}
	if status >= 400 && status != 401 {
		return fmt.Errorf("supabase logout failed with status %d", status)
	}
	return nil
}

// UpdatePassword sets a new password for the user identified by accessToken
// — used to complete the password-reset flow after the user follows the
// recovery link Supabase emailed them.
func (c *Client) UpdatePassword(ctx context.Context, accessToken, newPassword string) error {
	respBody, status, err := c.do(ctx, http.MethodPut, "/auth/v1/user", map[string]string{
		"password": newPassword,
	}, accessToken)
	if err != nil {
		return err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		return fmt.Errorf("supabase password update failed: %s", gErr.String())
	}
	return nil
}

// SendPasswordReset triggers Supabase's own password-reset email flow.
// redirectTo tells Supabase where the recovery link should land — without
// it, Supabase falls back to its project-level default, which may not be
// our /reset-password page.
func (c *Client) SendPasswordReset(ctx context.Context, email, redirectTo string) error {
	respBody, status, err := c.do(ctx, http.MethodPost, "/auth/v1/recover", map[string]string{
		"email":       email,
		"redirect_to": redirectTo,
	}, "")
	if err != nil {
		return err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		return fmt.Errorf("supabase password reset failed: %s", gErr.String())
	}
	return nil
}

// DeleteUser permanently deletes a user from Supabase Auth, using the
// project's service-role key rather than the anon key or a user access
// token — this is an admin-only GoTrue endpoint.
func (c *Client) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/auth/v1/admin/users/"+userID.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.serviceRoleKey)
	req.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		return fmt.Errorf("supabase delete user failed: %s", gErr.String())
	}
	return nil
}

// ResendVerificationEmail re-sends the signup confirmation email. redirectTo
// mirrors SignUp's — without it the link falls back to Supabase's
// project-level default instead of our login page.
func (c *Client) ResendVerificationEmail(ctx context.Context, email, redirectTo string) error {
	respBody, status, err := c.do(ctx, http.MethodPost, "/auth/v1/resend", map[string]string{
		"type": "signup", "email": email, "redirect_to": redirectTo,
	}, "")
	if err != nil {
		return err
	}
	if status >= 400 {
		var gErr gotrueError
		json.Unmarshal(respBody, &gErr)
		return fmt.Errorf("supabase resend verification failed: %s", gErr.String())
	}
	return nil
}
