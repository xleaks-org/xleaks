package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalOnlyRejectsForwardedRequestsWithoutTokenAuth(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	rr := httptest.NewRecorder()

	LocalOnly(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("proxied request should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestLocalOnlyAllowsForwardedRequestsWhenTokenAuthEnabled(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")
	rr := httptest.NewRecorder()
	called := false

	LocalOnly(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected proxied request to reach handler when token auth is enabled")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
