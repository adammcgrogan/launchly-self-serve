// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
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

	StripeSecretKey      string
	StripeWebhookSecret  string
	StripeStarterPriceID string
	StripeProPriceID     string

	ResendAPIKey string
	EmailFrom    string

	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromNumber string

	SuperadminPassword string
	CookieSigningKey   string // HMAC key for CSRF/flash cookies (not auth — auth uses Supabase JWTs)

	UmamiScriptURL string

	CloudflareAPIToken       string
	CloudflareZoneID         string
	CloudflareFallbackOrigin string // fixed hostname customer domains are CNAME'd to, e.g. "origin.launchly.ltd"

	AlertWebhookURL string // Slack/Discord/Google Chat incoming webhook posted to on log records at or above AlertMinLevel; unset disables alerting
	AlertMinLevel   string // minimum slog level to post to the webhook: "info", "warn", or "error" (default)

	GeminiAPIKey string // Google Gemini API key for AI-drafted site copy; unset disables the feature
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

		StripeSecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripeStarterPriceID: getEnv("STRIPE_STARTER_PRICE_ID", ""),
		StripeProPriceID:     getEnv("STRIPE_PRO_PRICE_ID", ""),

		ResendAPIKey: getEnv("RESEND_API_KEY", ""),
		EmailFrom:    getEnv("EMAIL_FROM", "noreply@launchly.ltd"),

		TwilioAccountSID: getEnv("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:  getEnv("TWILIO_AUTH_TOKEN", ""),
		TwilioFromNumber: getEnv("TWILIO_FROM_NUMBER", ""),

		SuperadminPassword: os.Getenv("SUPERADMIN_PASSWORD"),
		CookieSigningKey:   os.Getenv("COOKIE_SIGNING_KEY"),

		UmamiScriptURL: getEnv("UMAMI_SCRIPT_URL", ""),

		CloudflareAPIToken:       getEnv("CLOUDFLARE_API_TOKEN", ""),
		CloudflareZoneID:         getEnv("CLOUDFLARE_ZONE_ID", ""),
		CloudflareFallbackOrigin: getEnv("CLOUDFLARE_FALLBACK_ORIGIN", ""),

		AlertWebhookURL: getEnv("ALERT_WEBHOOK_URL", ""),
		AlertMinLevel:   getEnv("ALERT_MIN_LEVEL", "error"),

		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
	}

	required := map[string]string{
		"DATABASE_URL":        cfg.DatabaseURL,
		"SUPABASE_URL":        cfg.SupabaseURL,
		"SUPABASE_ANON_KEY":   cfg.SupabaseAnonKey,
		"SUPABASE_JWT_SECRET": cfg.SupabaseJWTSecret,
		"SUPERADMIN_PASSWORD": cfg.SuperadminPassword,
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

// AIContentAvailable reports whether AI-drafted site copy is available —
// the feature flag for the "Generate for me" button in the builder wizard.
func (c *Config) AIContentAvailable() bool {
	return c.GeminiAPIKey != ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
