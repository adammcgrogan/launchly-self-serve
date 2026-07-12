// Package cloudflare wraps the small slice of Cloudflare's Custom Hostnames
// (Cloudflare for SaaS) API needed for self-serve custom domains: creating a
// custom hostname, checking its verification/TLS status, and deleting it.
// Cloudflare fronts each hostname's TLS and proxies to one fixed origin
// (config.CloudflareFallbackOrigin) — Railway never sees customer domains.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiBase is a var (not const) so tests can point it at a local server.
var apiBase = "https://api.cloudflare.com/client/v4"

type Client struct {
	apiToken   string
	zoneID     string
	httpClient *http.Client
}

func New(apiToken, zoneID string) *Client {
	return &Client{apiToken: apiToken, zoneID: zoneID, httpClient: &http.Client{}}
}

// DNSRecord is one DNS record a customer needs to add, surfaced to the
// dashboard as setup instructions.
type DNSRecord struct {
	Type  string
	Name  string
	Value string
}

// Hostname is the state of a Cloudflare custom hostname.
type Hostname struct {
	ID                    string
	Status                string // hostname activation status: pending, active, ...
	SSLStatus             string // cert issuance status: pending_validation, pending_issuance, active, ...
	OwnershipVerification *DNSRecord
	SSLValidationRecords  []DNSRecord
}

// Active reports whether the hostname is fully live: routing verified and a
// certificate issued.
func (h Hostname) Active() bool {
	return h.Status == "active" && h.SSLStatus == "active"
}

// Failed reports whether Cloudflare has given up on this hostname.
func (h Hostname) Failed() bool {
	return h.SSLStatus == "failed" || h.Status == "moved"
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type apiError struct {
	Message string `json:"message"`
}

type hostnameResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	SSL    struct {
		Status            string `json:"status"`
		ValidationRecords []struct {
			TxtName  string `json:"txt_name"`
			TxtValue string `json:"txt_value"`
		} `json:"validation_records"`
	} `json:"ssl"`
	OwnershipVerification struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
}

func (r hostnameResult) toHostname() Hostname {
	h := Hostname{ID: r.ID, Status: r.Status, SSLStatus: r.SSL.Status}
	if r.OwnershipVerification.Name != "" {
		h.OwnershipVerification = &DNSRecord{
			Type: "TXT", Name: r.OwnershipVerification.Name, Value: r.OwnershipVerification.Value,
		}
	}
	for _, v := range r.SSL.ValidationRecords {
		if v.TxtName == "" {
			continue
		}
		h.SSLValidationRecords = append(h.SSLValidationRecords, DNSRecord{Type: "TXT", Name: v.TxtName, Value: v.TxtValue})
	}
	return h
}

// CreateCustomHostname registers hostname with Cloudflare for SaaS,
// returning its verification/routing state.
func (c *Client) CreateCustomHostname(ctx context.Context, hostname string) (*Hostname, error) {
	body, err := json.Marshal(map[string]any{
		"hostname": hostname,
		"ssl": map[string]any{
			"method": "txt",
			"type":   "dv",
		},
	})
	if err != nil {
		return nil, err
	}
	var result hostnameResult
	if err := c.do(ctx, http.MethodPost, "/zones/"+c.zoneID+"/custom_hostnames", body, &result); err != nil {
		return nil, err
	}
	h := result.toHostname()
	return &h, nil
}

// GetCustomHostname fetches the current verification/routing state of a
// previously created custom hostname.
func (c *Client) GetCustomHostname(ctx context.Context, cfID string) (*Hostname, error) {
	var result hostnameResult
	if err := c.do(ctx, http.MethodGet, "/zones/"+c.zoneID+"/custom_hostnames/"+cfID, nil, &result); err != nil {
		return nil, err
	}
	h := result.toHostname()
	return &h, nil
}

// DeleteCustomHostname removes a custom hostname from Cloudflare.
func (c *Client) DeleteCustomHostname(ctx context.Context, cfID string) error {
	return c.do(ctx, http.MethodDelete, "/zones/"+c.zoneID+"/custom_hostnames/"+cfID, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read cloudflare response: %w", err)
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}
	if !envelope.Success {
		if len(envelope.Errors) > 0 {
			return fmt.Errorf("cloudflare error: %s", envelope.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare error: status %d", resp.StatusCode)
	}
	if out == nil || len(envelope.Result) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
