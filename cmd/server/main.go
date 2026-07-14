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
	"github.com/adammcgrogan/launchly-self-serve/internal/supabase"
	"github.com/adammcgrogan/launchly-self-serve/internal/web"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(alert.New(slog.NewJSONHandler(os.Stdout, nil), cfg.AlertWebhookURL, alert.ParseLevel(cfg.AlertMinLevel))))

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

	accounts := service.NewAccounts(store, supa, mailer, baseURL)
	analytics := service.NewAnalytics(store, cfg.AnalyticsSalt)
	billing := service.NewBilling(store, pay, mailer, baseURL)
	sites := service.NewSites(store, billing)
	leads := service.NewLeads(store, mailer, sms)
	cron := service.NewCron(store, mailer, analytics, baseURL)

	cf := cloudflare.New(cfg.CloudflareAPIToken, cfg.CloudflareZoneID)
	domains := service.NewDomains(store, cf, cfg.CloudflareFallbackOrigin, cfg.Domain)

	secureCookies := !strings.Contains(cfg.Domain, "localhost")
	auth := middleware.NewAuth(cfg.SupabaseJWTSecret, supa, secureCookies)
	superadmin := middleware.NewSuperadmin(cfg.SuperadminPassword, cfg.CookieSigningKey, secureCookies)

	h, err := web.New(web.Deps{
		Cfg: cfg, Store: store,
		Accounts: accounts, Sites: sites, Billing: billing, Leads: leads, Analytics: analytics, Cron: cron, Domains: domains, AI: aiClient,
		Auth: auth, Superadmin: superadmin,
	})
	if err != nil {
		slog.Error("handler init failed", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
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

// contentSecurityPolicy is tuned for the Tailwind CDN build and inline
// <script>/<style> usage across the dashboard, auth, and site templates —
// none of it is nonce-based yet, so 'unsafe-inline' stays in for now.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com data:; " +
	"img-src 'self' https: data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'self'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// securityHeaders adds security-related HTTP response headers to every response.
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
