package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// UploadImage receives a single image file (multipart field "file") from the
// builder or editor, validates and stores it in Supabase Storage via the
// uploads service, and returns the resulting public URL as JSON. The browser
// then drops that URL into the logo_url / gallery fields, so the rest of the
// save flow is unchanged.
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	if !h.uploads.Available() {
		writeUploadError(w, http.StatusServiceUnavailable, "Image uploads aren't available right now.")
		return
	}
	// Cap the parsed body so a large upload can't exhaust server memory; the
	// service enforces the same limit on the decoded file.
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(service.MaxUploadBytes + 1<<20); err != nil {
		writeUploadError(w, http.StatusRequestEntityTooLarge, "That image is too large — please use one under 5 MB.")
		return
	}
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if !h.uploadLimiter.Allow(middleware.UserID(r).String()) {
		writeUploadError(w, http.StatusTooManyRequests, "Too many uploads — please wait a moment and try again.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, "No file was uploaded.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, service.MaxUploadBytes+1))
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, "Couldn't read the uploaded file.")
		return
	}

	// Trust the sniffed content type over the browser-supplied one so the
	// extension a client claims can't smuggle a non-image through.
	contentType := http.DetectContentType(data)

	url, err := h.uploads.UploadImage(r.Context(), middleware.UserID(r), contentType, data)
	if err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			writeUploadError(w, http.StatusBadRequest, verr.Message)
			return
		}
		if errors.Is(err, service.ErrUploadsUnavailable) {
			writeUploadError(w, http.StatusServiceUnavailable, "Image uploads aren't available right now.")
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	_ = header // filename is intentionally ignored — stored objects are named by UUID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": url})
}

func writeUploadError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
