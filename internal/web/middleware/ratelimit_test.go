package middleware

import (
	"net"
	"net/http"
	"testing"
)

func TestClientIP_UntrustedPeerIgnoresSpoofedHeader(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "1.2.3.4:5678",
		Header:     http.Header{},
	}
	r.Header.Set("CF-Connecting-IP", "9.9.9.9")
	r.Header.Set("X-Forwarded-For", "8.8.8.8")

	got := ClientIP(r)
	if got != "1.2.3.4" {
		t.Fatalf("expected RemoteAddr to be used for a non-Cloudflare peer, got %q", got)
	}
}

func TestClientIP_TrustedCloudflarePeerUsesHeader(t *testing.T) {
	r := &http.Request{
		// 172.64.0.1 falls within Cloudflare's published 172.64.0.0/13 range.
		RemoteAddr: "172.64.0.1:5678",
		Header:     http.Header{},
	}
	r.Header.Set("CF-Connecting-IP", "203.0.113.42")

	got := ClientIP(r)
	if got != "203.0.113.42" {
		t.Fatalf("expected CF-Connecting-IP to be trusted for a Cloudflare peer, got %q", got)
	}
}

func TestClientIP_TrustedCloudflarePeerFallsBackToXFF(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "104.16.1.1:5678",
		Header:     http.Header{},
	}
	r.Header.Set("X-Forwarded-For", "203.0.113.42, 10.0.0.1")

	got := ClientIP(r)
	if got != "203.0.113.42" {
		t.Fatalf("expected first X-Forwarded-For hop to be trusted for a Cloudflare peer, got %q", got)
	}
}

func TestClientIP_UntrustedPeerNoHeaders(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "203.0.113.5:9999",
		Header:     http.Header{},
	}

	got := ClientIP(r)
	if got != "203.0.113.5" {
		t.Fatalf("expected bare RemoteAddr, got %q", got)
	}
}

func TestIsCloudflareIP_IPv6Range(t *testing.T) {
	if !isCloudflareIP(net.ParseIP("2606:4700:1234::1")) {
		t.Fatal("expected address in 2606:4700::/32 to be recognized as Cloudflare")
	}
	if isCloudflareIP(net.ParseIP("2001:db8::1")) {
		t.Fatal("expected address outside Cloudflare ranges to not be recognized")
	}
}
