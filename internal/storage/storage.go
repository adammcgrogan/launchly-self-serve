// Package storage is a thin REST client for the Supabase Storage API, used to
// hold customer-uploaded images (site logos and gallery photos). Our Go server
// proxies the upload: the browser posts the file to a dashboard handler, which
// validates it and calls Upload here with the project's service-role key. The
// resulting public URL is stored in the site's logo_url / gallery fields
// exactly as a pasted URL would be.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client uploads objects to a single public Supabase Storage bucket.
type Client struct {
	baseURL    string // e.g. https://xyzcompany.supabase.co
	serviceKey string // service-role key — Storage writes bypass RLS
	bucket     string // public bucket the objects live in
	http       *http.Client
}

// New builds a Storage client. baseURL is the Supabase project URL (the same
// one the auth client uses); serviceKey is the service-role key; bucket is the
// name of a public bucket that must already exist.
func New(baseURL, serviceKey, bucket string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceKey: serviceKey,
		bucket:     bucket,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Configured reports whether the client has everything it needs to upload.
func (c *Client) Configured() bool {
	return c.baseURL != "" && c.serviceKey != "" && c.bucket != ""
}

// Upload stores data at objectPath within the bucket and returns its public
// URL. objectPath is a bucket-relative key like "user-id/uuid.png".
// contentType is sent verbatim as the object's Content-Type. Existing objects
// at the same path are overwritten.
func (c *Client) Upload(ctx context.Context, objectPath, contentType string, data []byte) (string, error) {
	objectPath = strings.TrimLeft(objectPath, "/")
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", c.baseURL, c.bucket, objectPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("apikey", c.serviceKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("cache-control", "3600")
	// Overwrite rather than 409 if the same key is uploaded twice — keys carry
	// a UUID so this only matters on a client retry of the exact same object.
	req.Header.Set("x-upsert", "true")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		return "", fmt.Errorf("supabase storage upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return fmt.Sprintf("%s/storage/v1/object/public/%s/%s", c.baseURL, c.bucket, objectPath), nil
}
