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

func TestIdentityExportRequiresPost(t *testing.T) {
	t.Parallel()

	router := NewRouter(&HandlerDeps{}, nil)

	getReq := httptest.NewRequest(http.MethodGet, "/api/identity/export", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusMethodNotAllowed && getRR.Code != http.StatusNotFound {
		t.Fatalf("GET /api/identity/export status = %d, want 404 or 405", getRR.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/identity/export", nil)
	postRR := httptest.NewRecorder()
	router.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusInternalServerError && postRR.Code != http.StatusNotFound {
		t.Fatalf("POST /api/identity/export status = %d, want 500 or 404", postRR.Code)
	}
}

func TestDMRoutesDisableCaching(t *testing.T) {
	t.Parallel()

	router := NewRouter(&HandlerDeps{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/dm", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

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

func TestNotificationRoutesDisableCaching(t *testing.T) {
	t.Parallel()

	router := NewRouter(&HandlerDeps{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

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
