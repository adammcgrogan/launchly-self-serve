package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteByURL_ForeignOrEmptyURLIsNoop(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "service-key", "photos")

	if err := c.DeleteByURL(context.Background(), ""); err != nil {
		t.Fatalf("empty URL: unexpected error: %v", err)
	}
	if err := c.DeleteByURL(context.Background(), "https://example.com/some/other/image.png"); err != nil {
		t.Fatalf("foreign URL: unexpected error: %v", err)
	}
	if called {
		t.Fatal("DeleteByURL should not call Storage for a URL it doesn't own")
	}
}

func TestDeleteByURL_OwnObjectDeletesByPath(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "service-key", "photos")
	url := srv.URL + "/storage/v1/object/public/photos/owner-id/abc.png"

	if err := c.DeleteByURL(context.Background(), url); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if want := "/storage/v1/object/photos/owner-id/abc.png"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
