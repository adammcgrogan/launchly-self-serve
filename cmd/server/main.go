// Command server is the entry point: loads config, wires every layer
// together, and starts the HTTP server plus background cron goroutines.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/ai"
	"github.com/adammcgrogan/launchly-self-serve/internal/alert"
	"github.com/adammcgrogan/launchly-self-serve/internal/cloudflare"
	"github.com/adammcgrogan/launchly-self-serve/internal/config"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/notify"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/storage"
	"github.com/adammcgrogan/launchly-self-serve/internal/supabase"
	"github.com/adammcgrogan/launchly-self-serve/internal/web"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
	"github.com/google/uuid"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	alertWebhooks := alert.Webhooks{
		Default: cfg.AlertWebhookURL,
		Info:    cfg.AlertWebhookURLInfo,
		Warn:    cfg.AlertWebhookURLWarn,
		Error:   cfg.AlertWebhookURLError,
	}
	slog.SetDefault(slog.New(alert.New(slog.NewJSONHandler(os.Stdout, nil), alertWebhooks, alert.ParseLevel(cfg.AlertMinLevel))))

	store, err := postgres.New(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database init failed", "error", err)
		os.Exit(1)
	}
	if err := store.Migrate(); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}

	supa := supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseAnonKey, cfg.SupabaseServiceRoleKey)
	mailer := email.New(cfg.ResendAPIKey, cfg.EmailFrom)
	sms := notify.NewSMSClient(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioFromNumber)
	pay := payment.New(cfg.StripeSecretKey, cfg.StripeWebhookSecret, cfg.StripeStarterPriceID, cfg.StripeProPriceID)
	aiClient := ai.New(cfg.GeminiAPIKey)

	baseURL := "https://" + cfg.Domain
	if strings.Contains(cfg.Domain, "localhost") {
		baseURL = "http://" + cfg.Domain
	}

	cf := cloudflare.New(cfg.CloudflareAPIToken, cfg.CloudflareZoneID)
	imageStore := storage.New(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.SupabaseStorageBucket)
	uploads := service.NewUploads(imageStore)

	accounts := service.NewAccounts(store, supa, mailer, baseURL)
	analytics := service.NewAnalytics(store, cfg.AnalyticsSalt)
	billing := service.NewBilling(store, pay, mailer, baseURL)
	sites := service.NewSites(store, billing, cf, uploads)
	leads := service.NewLeads(store, mailer, sms)
	cron := service.NewCron(store, mailer, analytics, billing, baseURL)

	if cfg.DemoOwnerUserID != "" {
		if ownerID, err := uuid.Parse(cfg.DemoOwnerUserID); err != nil {
			slog.Error("invalid DEMO_OWNER_USER_ID", "error", err)
		} else {
			// Backgrounded: each demo site is ~15 sequential DB round-trips,
			// which is too slow to block startup on — the server must bind
			// its port promptly or Railway/Cloudflare serve 502s in the gap.
			go func() {
				if err := sites.SeedDemoSites(context.Background(), ownerID); err != nil {
					slog.Error("seed demo sites failed", "error", err)
				}
			}()
		}
	}

	domains := service.NewDomains(store, cf, mailer, cfg.CloudflareFallbackOrigin, cfg.Domain, baseURL)
	members := service.NewMembers(store, mailer, baseURL)

	superadminSvc := service.NewSuperadmin(store)
	if err := superadminSvc.Bootstrap(context.Background(), cfg.SuperadminBootstrapEmail, cfg.SuperadminBootstrapPassword); err != nil {
		// Non-fatal: an admin account already existing (or bootstrap vars
		// left unset after the first admin is created) is the normal
		// steady state, so a bootstrap error shouldn't take the server
		// down — it just means superadmin login won't work until fixed.
		slog.Error("superadmin bootstrap failed", "error", err)
	}

	secureCookies := !strings.Contains(cfg.Domain, "localhost")
	auth := middleware.NewAuth(cfg.SupabaseJWTSecret, supa, secureCookies)
	superadmin := middleware.NewSuperadmin(cfg.CookieSigningKey, secureCookies)

	h, err := web.New(web.Deps{
		Cfg: cfg, Store: store,
		Accounts: accounts, Sites: sites, Billing: billing, Leads: leads, Analytics: analytics, Cron: cron, Domains: domains, Uploads: uploads, Members: members, AI: aiClient,
		Auth: auth, Superadmin: superadmin, SuperadminSvc: superadminSvc,
	})
	if err != nil {
		slog.Error("handler init failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticCacheHeaders(http.FileServer(http.Dir("web/static")))))
	h.RegisterRoutes(mux)

	finalHandler := middleware.RequestID(middleware.Recover(h.RenderError, loggingMiddleware(securityHeaders(web.SubdomainRouter(cfg.Domain, h, mux)))))

	cron.Start()

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      finalHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		slog.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("listening", "addr", cfg.Addr, "domain", cfg.Domain)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// contentSecurityPolicy is tuned for the inline <script>/<style> usage
// across the dashboard, auth, and site templates — none of it is
// nonce-based yet, so 'unsafe-inline' stays in for now.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com data:; " +
	"img-src 'self' https: data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// securityHeaders adds security-related HTTP response headers to every response.
// staticCacheHeaders sets Cache-Control on static assets so repeat visitors
// stop re-validating every file with a conditional request. Requests
// carrying the render package's ?v=<hash> cache-buster (internal/web/render.go's
// asset template func) are content-addressed — a deploy that changes the
// file changes the URL — so those get a far-future immutable cache. Anything
// else (e.g. an unversioned image referenced directly) gets a moderate
// max-age instead, since the same URL could later serve different content.
func staticCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		host := strings.Split(r.Host, ":")[0]
		if host != "localhost" && host != "127.0.0.1" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request with a request ID, method, host, path,
// status, response size, and duration. The health-check endpoint is skipped
// so platform liveness probes don't drown out real traffic in the logs.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		slog.Info("request",
			"request_id", middleware.GetRequestID(r),
			"method", r.Method, "host", r.Host, "path", r.URL.Path,
			"status", rec.status, "bytes", rec.bytes,
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"ip", middleware.ClientIP(r),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}
