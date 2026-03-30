package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityRoutesDisableCaching(t *testing.T) {
	t.Parallel()

	router := NewRouter(&HandlerDeps{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/identity/active", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store, max-age=0")
	}
	if got := rr.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	if got := rr.Header().Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q, want 0", got)
	}
}

func TestNonIdentityRoutesRemainCacheNeutral(t *testing.T) {
	t.Parallel()

	router := NewRouter(&HandlerDeps{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/node/status", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control = %q, want empty", got)
	}
}
