package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("test-token", "test-zone")
	// point the client at the test server instead of api.cloudflare.com
	origBase := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = origBase })
	return c
}

func TestCreateCustomHostname(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/zones/test-zone/custom_hostnames" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q", got)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["hostname"] != "example.com" {
			t.Fatalf("hostname = %v", body["hostname"])
		}
		w.Write([]byte(`{
			"success": true,
			"result": {
				"id": "cf123",
				"status": "pending",
				"ssl": {"status": "pending_validation", "validation_records": [{"txt_name": "_acme.example.com", "txt_value": "abc"}]},
				"ownership_verification": {"type": "txt", "name": "_cf-custom-hostname.example.com", "value": "token123"}
			}
		}`))
	})

	h, err := c.CreateCustomHostname(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("CreateCustomHostname: %v", err)
	}
	if h.ID != "cf123" || h.Status != "pending" || h.SSLStatus != "pending_validation" {
		t.Fatalf("unexpected hostname: %+v", h)
	}
	if h.OwnershipVerification == nil || h.OwnershipVerification.Value != "token123" {
		t.Fatalf("ownership verification not parsed: %+v", h.OwnershipVerification)
	}
	if len(h.SSLValidationRecords) != 1 || h.SSLValidationRecords[0].Value != "abc" {
		t.Fatalf("ssl validation records not parsed: %+v", h.SSLValidationRecords)
	}
}

func TestHostnameActiveAndFailed(t *testing.T) {
	cases := []struct {
		h          Hostname
		wantActive bool
		wantFailed bool
	}{
		{Hostname{Status: "active", SSLStatus: "active"}, true, false},
		{Hostname{Status: "pending", SSLStatus: "pending_validation"}, false, false},
		{Hostname{Status: "pending", SSLStatus: "failed"}, false, true},
		{Hostname{Status: "moved", SSLStatus: "active"}, false, true},
	}
	for _, tc := range cases {
		if got := tc.h.Active(); got != tc.wantActive {
			t.Errorf("Active() for %+v = %v, want %v", tc.h, got, tc.wantActive)
		}
		if got := tc.h.Failed(); got != tc.wantFailed {
			t.Errorf("Failed() for %+v = %v, want %v", tc.h, got, tc.wantFailed)
		}
	}
}

func TestAPIErrorSurfaced(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success": false, "errors": [{"message": "hostname already exists"}]}`))
	})
	_, err := c.CreateCustomHostname(context.Background(), "example.com")
	if err == nil || err.Error() != "cloudflare error: hostname already exists" {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteCustomHostname(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/zones/test-zone/custom_hostnames/cf123" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"success": true, "result": {}}`))
	})
	if err := c.DeleteCustomHostname(context.Background(), "cf123"); err != nil {
		t.Fatalf("DeleteCustomHostname: %v", err)
	}
}
