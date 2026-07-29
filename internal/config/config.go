// Package config loads application configuration from environment variables.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the server.
type Config struct {
	Addr   string
	Domain string // e.g. "launchly.ltd"

	DatabaseURL string // Supabase Postgres connection string

	SupabaseURL            string
	SupabaseAnonKey        string
	SupabaseServiceRoleKey string
	SupabaseJWTSecret      string
	SupabaseStorageBucket  string // public Storage bucket for user-uploaded logo/gallery images; unset disables uploads

	StripeSecretKey      string
	StripeWebhookSecret  string
	StripeStarterPriceID string
	StripeProPriceID     string

	ResendAPIKey string
	EmailFrom    string

	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string

	// SuperadminBootstrapEmail/Password seed the first per-admin superadmin
	// account (see #94) on a fresh environment where superadmin_admins is
	// empty — see service.Superadmin.Bootstrap. Once at least one admin
	// account exists these are ignored, so they're safe to leave set
	// permanently; additional admins beyond the first are added directly
	// in the DB (no self-serve "add admin" UI yet).
	SuperadminBootstrapEmail    string
	SuperadminBootstrapPassword string
	CookieSigningKey            string // HMAC key for CSRF/flash cookies (not auth — auth uses Supabase JWTs)
	AnalyticsSalt               string // salts the visitor-IP hash; independent of CookieSigningKey so rotating one doesn't affect the other

	CloudflareAPIToken       string
	CloudflareZoneID         string
	CloudflareFallbackOrigin string // fixed hostname customer domains are CNAME'd to, e.g. "origin.launchly.ltd"

	AlertWebhookURL      string // default Discord webhook posted to on log records at or above AlertMinLevel; unset disables alerting (unless a per-level override below is set)
	AlertWebhookURLInfo  string // overrides AlertWebhookURL for info-level records; unset falls back to AlertWebhookURL
	AlertWebhookURLWarn  string // overrides AlertWebhookURL for warn-level records; unset falls back to AlertWebhookURL
	AlertWebhookURLError string // overrides AlertWebhookURL for error-level records; unset falls back to AlertWebhookURL
	AlertMinLevel        string // minimum slog level to post to the webhook(s): "info", "warn", or "error" (default)

	GeminiAPIKey string // Google Gemini API key for AI-drafted site copy; unset disables the feature

	// DemoOwnerUserID is the Supabase auth user ID that owns the seeded
	// showcase demo sites (one per template, see service.Sites.SeedDemoSites).
	// Unset skips seeding — there's no demo owner account to attach them to.
	DemoOwnerUserID string
}

// Load reads configuration from the environment, loading a local .env file
// first if present (no-op in production where env vars are set by the platform).
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Addr:   getEnv("ADDR", ":8080"),
		Domain: getEnv("DOMAIN", "launchly.ltd"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		SupabaseURL:            os.Getenv("SUPABASE_URL"),
		SupabaseAnonKey:        os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseJWTSecret:      os.Getenv("SUPABASE_JWT_SECRET"),
		SupabaseStorageBucket:  getEnv("SUPABASE_STORAGE_BUCKET", ""),

		StripeSecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripeStarterPriceID: getEnv("STRIPE_STARTER_PRICE_ID", ""),
		StripeProPriceID:     getEnv("STRIPE_PRO_PRICE_ID", ""),

		ResendAPIKey: getEnv("RESEND_API_KEY", ""),
		EmailFrom:    getEnv("EMAIL_FROM", "Launchly <noreply@launchly.ltd>"),

		TwilioAccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioFromNumber: getEnv("TWILIO_FROM_NUMBER", ""),

		SuperadminBootstrapEmail:    getEnv("SUPERADMIN_BOOTSTRAP_EMAIL", ""),
		SuperadminBootstrapPassword: getEnv("SUPERADMIN_BOOTSTRAP_PASSWORD", ""),
		CookieSigningKey:            os.Getenv("COOKIE_SIGNING_KEY"),
		AnalyticsSalt:               os.Getenv("ANALYTICS_SALT"),

		CloudflareAPIToken:       getEnv("CLOUDFLARE_API_TOKEN", ""),
		CloudflareZoneID:         getEnv("CLOUDFLARE_ZONE_ID", ""),
		CloudflareFallbackOrigin: getEnv("CLOUDFLARE_FALLBACK_ORIGIN", ""),

		AlertWebhookURL:      getEnv("ALERT_WEBHOOK_URL", ""),
		AlertWebhookURLInfo:  getEnv("ALERT_WEBHOOK_URL_INFO", ""),
		AlertWebhookURLWarn:  getEnv("ALERT_WEBHOOK_URL_WARN", ""),
		AlertWebhookURLError: getEnv("ALERT_WEBHOOK_URL_ERROR", ""),
		AlertMinLevel:        getEnv("ALERT_MIN_LEVEL", "error"),

		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),

		DemoOwnerUserID: getEnv("DEMO_OWNER_USER_ID", ""),
	}

	if cfg.AnalyticsSalt == "" {
		// Fall back to a value derived from CookieSigningKey rather than
		// reusing it directly, so visitor-hash salting is still distinct
		// from CSRF/session signing even when ANALYTICS_SALT isn't set.
		slog.Warn("ANALYTICS_SALT not set — deriving a fallback from COOKIE_SIGNING_KEY; set ANALYTICS_SALT explicitly so rotating COOKIE_SIGNING_KEY doesn't re-bucket visitor hashes")
		sum := sha256.Sum256([]byte("analytics-salt-v1:" + cfg.CookieSigningKey))
		cfg.AnalyticsSalt = hex.EncodeToString(sum[:])
	}

	required := map[string]string{
		"DATABASE_URL":        cfg.DatabaseURL,
		"SUPABASE_URL":        cfg.SupabaseURL,
		"SUPABASE_ANON_KEY":   cfg.SupabaseAnonKey,
		"SUPABASE_JWT_SECRET": cfg.SupabaseJWTSecret,
		"COOKIE_SIGNING_KEY":  cfg.CookieSigningKey,
	}
	for key, val := range required {
		if val == "" {
			return nil, fmt.Errorf("required env var %s not set", key)
		}
	}

	return cfg, nil
}

// SMSAlertsAvailable reports whether Twilio is configured — the feature
// flag for SMS lead alerts. Left unset until we're ready to pay for Twilio,
// at which point setting the env vars turns the feature on with no code
// change.
func (c *Config) SMSAlertsAvailable() bool {
	return c.TwilioAccountSID != "" && c.TwilioAuthToken != "" && c.TwilioFromNumber != ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
