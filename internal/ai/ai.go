// Package ai drafts starting site copy (tagline, about text, call-to-action)
// from a business name and type using Google's Gemini API. It is entirely
// optional: Client.Configured reports false whenever no API key is set (the
// default), and callers skip offering the feature rather than making a
// doomed API call — same "unset key = feature off" pattern as
// internal/notify for Twilio.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// model is a small, free-tier-eligible Gemini model — plenty for drafting a
// few sentences of marketing copy, and cheap enough to run per-request with
// no caching.
const model = "gemini-flash-lite-latest"

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Configured reports whether a Gemini API key is set.
func (c *Client) Configured() bool {
	return c.apiKey != ""
}

// SiteCopy is a draft set of content fields for a new site, meant to be
// reviewed and edited by the owner before saving — never saved as-is.
type SiteCopy struct {
	Tagline string `json:"tagline"`
	About   string `json:"about"`
	CTAText string `json:"cta_text"`
}

type generateRequest struct {
	Contents         []content        `json:"contents"`
	GenerationConfig generationConfig `json:"generationConfig"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generationConfig struct {
	ResponseMIMEType string `json:"responseMimeType"`
	ResponseSchema   schema `json:"responseSchema"`
}

type schema struct {
	Type       string            `json:"type"`
	Properties map[string]schema `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

var siteCopySchema = schema{
	Type: "OBJECT",
	Properties: map[string]schema{
		"tagline":  {Type: "STRING"},
		"about":    {Type: "STRING"},
		"cta_text": {Type: "STRING"},
	},
	Required: []string{"tagline", "about", "cta_text"},
}

// GenerateSiteCopy drafts a tagline, about paragraph, and call-to-action
// button text for a small business site from just its name and type.
func (c *Client) GenerateSiteCopy(ctx context.Context, businessName, businessType string) (SiteCopy, error) {
	prompt := fmt.Sprintf(`You are writing starting content for a small business's one-page website. Business name: %q. Business type: %q.

Draft:
- tagline: a short, punchy tagline (under 12 words).
- about: 2-3 warm, plain-spoken sentences introducing the business to a potential customer. Don't invent specific facts (years in business, awards, locations) that weren't given.
- cta_text: 2-4 words for a call-to-action button, e.g. "Get a Quote" or "Book Now".

Return only the JSON fields requested.`, businessName, businessType)

	reqBody := generateRequest{
		Contents: []content{{Parts: []part{{Text: prompt}}}},
		GenerationConfig: generationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   siteCopySchema,
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return SiteCopy{}, err
	}

	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return SiteCopy{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SiteCopy{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SiteCopy{}, err
	}
	if resp.StatusCode >= 400 {
		return SiteCopy{}, fmt.Errorf("gemini api error: %s: %s", resp.Status, body)
	}

	var out generateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return SiteCopy{}, fmt.Errorf("decode gemini response: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return SiteCopy{}, fmt.Errorf("gemini returned no content")
	}

	var copy SiteCopy
	if err := json.Unmarshal([]byte(out.Candidates[0].Content.Parts[0].Text), &copy); err != nil {
		return SiteCopy{}, fmt.Errorf("parse generated copy: %w", err)
	}
	return copy, nil
}
