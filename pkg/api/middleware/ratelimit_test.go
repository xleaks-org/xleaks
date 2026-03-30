package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestExtractIPIgnoresForwardedHeadersFromDirectClients(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "http://example.test/api/node/status", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.Header.Set("X-Real-IP", "198.51.100.8")

	if got := extractIP(req); got != "203.0.113.10" {
		t.Fatalf("extractIP() = %q, want %q", got, "203.0.113.10")
	}
}

func TestExtractIPTrustsForwardedHeadersFromLoopbackProxy(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "http://example.test/api/node/status", nil)
	req.RemoteAddr = "127.0.0.1:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 127.0.0.1")

	if got := extractIP(req); got != "198.51.100.7" {
		t.Fatalf("extractIP() = %q, want %q", got, "198.51.100.7")
	}
}

func TestExtractIPFallsBackWhenForwardedHeaderIsInvalid(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("GET", "http://example.test/api/node/status", nil)
	req.RemoteAddr = "127.0.0.1:4567"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "198.51.100.8")

	if got := extractIP(req); got != "198.51.100.8" {
		t.Fatalf("extractIP() = %q, want %q", got, "198.51.100.8")
	}
}
