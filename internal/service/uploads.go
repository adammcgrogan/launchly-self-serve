package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/adammcgrogan/launchly-self-serve/internal/storage"
	"github.com/google/uuid"
)

// MaxUploadBytes caps how large a single logo/gallery image may be. Kept small
// on purpose — these are website photos, not print assets, and the file is
// buffered in the server's memory while it's proxied to Storage.
const MaxUploadBytes = 5 << 20 // 5 MiB

// ErrUploadsUnavailable is returned when image uploads aren't configured (no
// Storage bucket) — callers should fall back to the URL-only fields.
var ErrUploadsUnavailable = errors.New("image uploads are not available")

// allowedImageTypes maps an accepted MIME type to the file extension used for
// the stored object. Anything not in this map is rejected.
var allowedImageTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
}

// Uploads handles customer image uploads (logos, gallery photos), storing them
// in Supabase Storage and returning the public URL to save alongside the site.
type Uploads struct {
	storage *storage.Client
}

func NewUploads(store *storage.Client) *Uploads {
	return &Uploads{storage: store}
}

// Available reports whether uploads are configured and usable.
func (u *Uploads) Available() bool {
	return u.storage != nil && u.storage.Configured()
}

// UploadImage validates and stores an image for the given owner, returning its
// public URL. contentType must be a recognised image type; data must be within
// MaxUploadBytes. Objects are namespaced by owner so a customer's uploads stay
// grouped and can't collide with another account's.
func (u *Uploads) UploadImage(ctx context.Context, ownerID uuid.UUID, contentType string, data []byte) (string, error) {
	if !u.Available() {
		return "", ErrUploadsUnavailable
	}
	if len(data) == 0 {
		return "", &ValidationError{Message: "The file is empty."}
	}
	if len(data) > MaxUploadBytes {
		return "", &ValidationError{Message: "That image is too large — please use one under 5 MB."}
	}
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return "", &ValidationError{Message: "That file type isn't supported — please upload a JPG, PNG, WebP, or GIF."}
	}

	objectPath := fmt.Sprintf("%s/%s.%s", ownerID.String(), uuid.NewString(), ext)
	return u.storage.Upload(ctx, objectPath, contentType, data)
}

// DeleteImage removes a previously-uploaded image from Storage, identified
// by the public URL UploadImage returned for it. URLs that aren't one of
// ours (an externally-hosted image a site owner pasted in directly) are
// left untouched. A no-op if uploads aren't configured.
func (u *Uploads) DeleteImage(ctx context.Context, publicURL string) error {
	if !u.Available() {
		return nil
	}
	return u.storage.DeleteByURL(ctx, publicURL)
}
