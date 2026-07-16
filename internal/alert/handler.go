// Package alert wraps an slog.Handler so that log records at or above a
// configurable minimum level also get posted to a Discord incoming
// webhook (payload shape is Discord's {"content": "..."} — not Slack's
// {"text": "..."}, the two aren't interchangeable). It's entirely
// optional: with no webhook URL configured, Handler behaves exactly like
// the handler it wraps — same "unset key = feature off" pattern as
// internal/notify and internal/email. This gives production alerting
// (errors, or general logs if the level is lowered) without paying for a
// hosted APM vendor.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Handler wraps an slog.Handler and posts records at or above minLevel to a webhook.
type Handler struct {
	next       slog.Handler
	webhookURL string
	minLevel   slog.Level
	httpClient *http.Client
}

// New wraps next so records at or above minLevel are also posted to
// webhookURL. If webhookURL is empty, the returned handler just delegates
// to next.
func New(next slog.Handler, webhookURL string, minLevel slog.Level) *Handler {
	return &Handler{
		next:       next,
		webhookURL: webhookURL,
		minLevel:   minLevel,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// skipMessages are high-volume or low-value log lines that should never be
// forwarded to the webhook regardless of level — per-request access logs
// (message "request", logged once per HTTP request in cmd/server/main.go)
// would otherwise flood the channel at ALERT_MIN_LEVEL=info, and "listening"
// (logged once at startup) is routine noise rather than an alertable event.
var skipMessages = map[string]bool{
	"request":   true,
	"listening": true,
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.webhookURL != "" && r.Level >= h.minLevel && !skipMessages[r.Message] {
		h.notify(r)
	}
	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), webhookURL: h.webhookURL, minLevel: h.minLevel, httpClient: h.httpClient}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), webhookURL: h.webhookURL, minLevel: h.minLevel, httpClient: h.httpClient}
}

// ParseLevel maps a config string ("info", "warn", "error", ...) to an
// slog.Level, defaulting to LevelError for empty or unrecognized input so a
// misconfigured value fails safe toward less noise, not more.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

// discordEmbedColor maps an slog level to a Discord embed sidebar color.
func discordEmbedColor(level slog.Level) int {
	switch {
	case level >= slog.LevelError:
		return 0xE74C3C // red
	case level >= slog.LevelWarn:
		return 0xF1C40F // yellow
	case level >= slog.LevelInfo:
		return 0x3498DB // blue
	default:
		return 0x95A5A6 // gray (debug)
	}
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title     string              `json:"title"`
	Color     int                 `json:"color"`
	Fields    []discordEmbedField `json:"fields,omitempty"`
	Timestamp string              `json:"timestamp"`
}

// notify posts the record to the webhook in the background so logging
// never blocks on a slow/unreachable webhook endpoint.
func (h *Handler) notify(r slog.Record) {
	var fields []discordEmbedField
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, discordEmbedField{Name: a.Key, Value: a.Value.String(), Inline: true})
		return true
	})

	embed := discordEmbed{
		Title:     r.Message,
		Color:     discordEmbedColor(r.Level),
		Fields:    fields,
		Timestamp: r.Time.Format(time.RFC3339),
	}

	payload, err := json.Marshal(map[string]any{"embeds": []discordEmbed{embed}})
	if err != nil {
		return
	}

	go func(url string, body []byte, client *http.Client) {
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			return
		}
		resp.Body.Close()
	}(h.webhookURL, payload, h.httpClient)
}
