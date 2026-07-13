// Package alert wraps an slog.Handler so that Error-level (and above) log
// records also get posted to a chat webhook (Slack/Discord/Google Chat all
// accept the same {"text": "..."} payload shape). It's entirely optional:
// with no webhook URL configured, Handler behaves exactly like the handler
// it wraps — same "unset key = feature off" pattern as internal/notify and
// internal/email. This gives production error alerting without paying for
// a hosted APM vendor.
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

// Handler wraps an slog.Handler and posts Error-level+ records to a webhook.
type Handler struct {
	next       slog.Handler
	webhookURL string
	httpClient *http.Client
}

// New wraps next so Error-level+ records are also posted to webhookURL. If
// webhookURL is empty, the returned handler just delegates to next.
func New(next slog.Handler, webhookURL string) *Handler {
	return &Handler{
		next:       next,
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.webhookURL != "" && r.Level >= slog.LevelError {
		h.notify(r)
	}
	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), webhookURL: h.webhookURL, httpClient: h.httpClient}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), webhookURL: h.webhookURL, httpClient: h.httpClient}
}

// notify posts the record to the webhook in the background so logging
// never blocks on a slow/unreachable webhook endpoint.
func (h *Handler) notify(r slog.Record) {
	var fields []string
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, a.Key+"="+a.Value.String())
		return true
	})

	text := "🚨 " + r.Message
	if len(fields) > 0 {
		text += " (" + strings.Join(fields, ", ") + ")"
	}

	payload, err := json.Marshal(map[string]string{"text": text})
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
