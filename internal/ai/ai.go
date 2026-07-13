// Package ai drafts starting site copy (tagline, about text, service
// descriptions) using Google's Gemini API. It is entirely optional:
// Client.Configured reports false whenever no API key is set (the
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
// sentence or two of marketing copy, and cheap enough to run per-request
// with no caching.
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

// GenerateTagline drafts a short, punchy tagline for a business's site.
func (c *Client) GenerateTagline(ctx context.Context, businessName, businessType string) (string, error) {
	prompt := fmt.Sprintf(`Write a short, punchy tagline (under 12 words) for a small business's one-page website. Business name: %q. Business type: %q. Return only the tagline text, nothing else.`, businessName, businessType)
	return c.generate(ctx, prompt)
}

// GenerateAbout drafts a short "about" paragraph for a business's site.
func (c *Client) GenerateAbout(ctx context.Context, businessName, businessType string) (string, error) {
	prompt := fmt.Sprintf(`Write 2-3 warm, plain-spoken sentences introducing a small business to a potential customer, for the "about" section of its one-page website. Business name: %q. Business type: %q. Don't invent specific facts (years in business, awards, locations) that weren't given. Return only the about text, nothing else.`, businessName, businessType)
	return c.generate(ctx, prompt)
}

// GenerateServiceDescription drafts a short description of one service a
// business offers.
func (c *Client) GenerateServiceDescription(ctx context.Context, businessName, businessType, serviceName string) (string, error) {
	prompt := fmt.Sprintf(`Write a short description (under 20 words) of one service offered by a small business, for its one-page website. Business name: %q. Business type: %q. Service: %q. Return only the description text, nothing else.`, businessName, businessType, serviceName)
	return c.generate(ctx, prompt)
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
	Type string `json:"type"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// generate sends prompt to Gemini and returns the plain-text response,
// requesting a JSON string back so the model can't wrap the answer in
// preamble or markdown.
func (c *Client) generate(ctx context.Context, prompt string) (string, error) {
	reqBody := generateRequest{
		Contents: []content{{Parts: []part{{Text: prompt}}}},
		GenerationConfig: generationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   schema{Type: "STRING"},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini api error: %s: %s", resp.Status, body)
	}

	var out generateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode gemini response: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	var text string
	if err := json.Unmarshal([]byte(out.Candidates[0].Content.Parts[0].Text), &text); err != nil {
		return "", fmt.Errorf("parse generated text: %w", err)
	}
	return text, nil
}
