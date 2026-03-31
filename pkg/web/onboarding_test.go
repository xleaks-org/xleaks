package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

func testOnboardingHandler(t *testing.T, holder *identity.Holder) *Handler {
	t.Helper()

	sessions := NewSessionManager()
	t.Cleanup(func() {
		sessions.Stop()
	})

	handler, err := NewHandler(nil, holder, nil, sessions)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func testBlockedIdentityHolder(t *testing.T) (*identity.Holder, string) {
	t.Helper()

	blockedPath := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedPath, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return identity.NewHolder(blockedPath), blockedPath
}

func TestHandleCreateDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	holder, blockedPath := testBlockedIdentityHolder(t)
	h := testOnboardingHandler(t, holder)

	req := httptest.NewRequest(http.MethodPost, "/onboarding/create", strings.NewReader("passphrase=correcthorsebattery&confirm=correcthorsebattery"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.handleCreate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Failed to create identity") {
		t.Fatalf("body = %q, want generic create failure", body)
	}
	if strings.Contains(body, blockedPath) || strings.Contains(body, "not a directory") {
		t.Fatalf("body leaked backend create failure details: %q", body)
	}
}

func TestHandleImportDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	holder, blockedPath := testBlockedIdentityHolder(t)
	h := testOnboardingHandler(t, holder)

	mnemonic, err := identity.GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/onboarding/import", strings.NewReader("mnemonic="+strings.ReplaceAll(mnemonic, " ", "+")+"&passphrase=correcthorsebattery"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.handleImport(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Failed to import identity") {
		t.Fatalf("body = %q, want generic import failure", body)
	}
	if strings.Contains(body, blockedPath) || strings.Contains(body, "not a directory") {
		t.Fatalf("body leaked backend import failure details: %q", body)
	}
}
