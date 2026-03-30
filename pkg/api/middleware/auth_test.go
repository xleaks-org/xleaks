package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestTokenAuthRejectsQueryTokenOnNonWebSocketRequests(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/status?token=secret", nil)
	rr := httptest.NewRecorder()

	TokenAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("non-websocket query token should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestTokenAuthAllowsQueryTokenOnWebSocketHandshake(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/ws?token=secret", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rr := httptest.NewRecorder()
	called := false

	TokenAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("websocket query token should reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestTokenAuthAllowsWebSocketTicketOnHandshake(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/ws?ws_ticket=ticket-123", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	rr := httptest.NewRecorder()
	called := false

	TokenAuthWithWebSocketTicket("secret", func(ticket string) bool {
		return ticket == "ticket-123"
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("websocket ticket should reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestSaveTokenCreatesOwnerOnlyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tokens", "api.token")
	if err := SaveToken("test-token", path); err != nil {
		t.Fatalf("SaveToken() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "test-token" {
		t.Fatalf("token contents = %q, want %q", string(data), "test-token")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveTokenTightensExistingPermissions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "api.token")
	if err := os.WriteFile(path, []byte("old-token"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := SaveToken("new-token", path); err != nil {
		t.Fatalf("SaveToken() error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveTokenCleansTempFileOnFinalizeFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tokens", "api.token")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	if err := SaveToken("test-token", path); err == nil {
		t.Fatal("SaveToken() should fail when final path is a directory")
	}

	tempMatches, err := filepath.Glob(filepath.Join(filepath.Dir(path), filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatalf("Glob() error: %v", err)
	}
	if len(tempMatches) != 0 {
		t.Fatalf("expected no temporary token files after failed finalize, got %v", tempMatches)
	}
}
