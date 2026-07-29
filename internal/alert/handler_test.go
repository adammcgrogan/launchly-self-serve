package alert

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHandlerRoutesByLevel(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}

	newServer := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[name]++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
	}

	defaultSrv := newServer("default")
	warnSrv := newServer("warn")
	errorSrv := newServer("error")
	defer defaultSrv.Close()
	defer warnSrv.Close()
	defer errorSrv.Close()

	webhooks := Webhooks{
		Default: defaultSrv.URL,
		Warn:    warnSrv.URL,
		Error:   errorSrv.URL,
	}

	h := New(slog.NewJSONHandler(discard{}, nil), webhooks, slog.LevelInfo)
	h.httpClient = &http.Client{Timeout: 2 * time.Second}

	log := func(level slog.Level, msg string) {
		r := slog.NewRecord(time.Now(), level, msg, 0)
		_ = h.Handle(context.Background(), r)
	}

	log(slog.LevelInfo, "info msg")   // -> default (no info override)
	log(slog.LevelWarn, "warn msg")   // -> warn
	log(slog.LevelError, "error msg") // -> error
	log(slog.LevelInfo, "request")    // skipped message, no post
	log(slog.LevelDebug, "debug msg") // below minLevel, no post

	// notify posts in a goroutine; give it a moment to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		total := hits["default"] + hits["warn"] + hits["error"]
		mu.Unlock()
		if total >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["default"] != 1 {
		t.Errorf("default webhook hits = %d, want 1", hits["default"])
	}
	if hits["warn"] != 1 {
		t.Errorf("warn webhook hits = %d, want 1", hits["warn"])
	}
	if hits["error"] != 1 {
		t.Errorf("error webhook hits = %d, want 1", hits["error"])
	}
}

func TestWebhooksForLevel(t *testing.T) {
	w := Webhooks{Default: "default", Warn: "warn", Error: "error"}

	cases := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "default"},
		{slog.LevelInfo, "default"}, // no Info override set
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
	}
	for _, c := range cases {
		if got := w.forLevel(c.level); got != c.want {
			t.Errorf("forLevel(%v) = %q, want %q", c.level, got, c.want)
		}
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
