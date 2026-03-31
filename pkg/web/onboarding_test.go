package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

func testOnboardingHandler(t *testing.T, holder *identity.Holder) *Handler {
	t.Helper()

	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB() error = %v", err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}

	sessions := NewSessionManager()
	t.Cleanup(func() {
		sessions.Stop()
		db.Close()
	})

	handler, err := NewHandler(db, holder, feed.NewTimeline(db, holder), sessions)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	cfg := config.DefaultConfig()
	handler.SetConfig(cfg, filepath.Join(dir, "config.toml"))
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

	req := httptest.NewRequest(http.MethodPost, "/onboarding/create", strings.NewReader("passphrase=correcthorsebattery&confirm=correcthorsebattery&storage_limit_gb=1"))
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

func TestHandleCreateRequiresMinimumStorageContribution(t *testing.T) {
	t.Parallel()

	holder := identity.NewHolder(filepath.Join(t.TempDir(), "identities"))
	h := testOnboardingHandler(t, holder)

	req := httptest.NewRequest(http.MethodPost, "/onboarding/create", strings.NewReader("passphrase=correcthorsebattery&confirm=correcthorsebattery&storage_limit_gb=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.handleCreate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Storage contribution must be at least 1 GB") {
		t.Fatalf("body = %q, want storage contribution validation", body)
	}
	if holder.HasIdentity() {
		t.Fatal("expected identity creation to be blocked when storage contribution is below minimum")
	}
}

func TestHandleCreatePersistsStorageContribution(t *testing.T) {
	t.Parallel()

	holder := identity.NewHolder(filepath.Join(t.TempDir(), "identities"))
	h := testOnboardingHandler(t, holder)

	var gotLimit int64
	h.SetStorageLimitChangeFunc(func(limit int64) {
		gotLimit = limit
	})

	req := httptest.NewRequest(http.MethodPost, "/onboarding/create", strings.NewReader("passphrase=correcthorsebattery&confirm=correcthorsebattery&storage_limit_gb=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.handleCreate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Save Your Seed Phrase") {
		t.Fatalf("body = %q, want seed-phrase step", body)
	}
	if !holder.HasIdentity() {
		t.Fatal("expected identity to be created")
	}
	if h.cfg == nil || h.cfg.Node.MaxStorageGB != 1 {
		t.Fatalf("in-memory storage_limit_gb = %v, want 1", h.cfg)
	}
	if gotLimit != int64(1024*1024*1024) {
		t.Fatalf("runtime storage limit callback = %d, want %d", gotLimit, int64(1024*1024*1024))
	}

	loaded, err := config.Load(h.cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Node.MaxStorageGB != 1 {
		t.Fatalf("saved storage_limit_gb = %d, want 1", loaded.Node.MaxStorageGB)
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
