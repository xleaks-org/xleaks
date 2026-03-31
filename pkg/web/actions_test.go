package web

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

func TestToggleThemeCookieIsReadableByClient(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "http://example.test/settings/toggle-theme", nil)
	rr := httptest.NewRecorder()

	h.handleToggleTheme(rr, req)

	cookie := findResponseCookie(rr.Result().Cookies(), "theme")
	if cookie == nil {
		t.Fatal("expected theme cookie")
	}
	if cookie.HttpOnly {
		t.Fatal("expected theme cookie to be readable by client scripts")
	}
	if cookie.Secure {
		t.Fatal("expected http theme cookie to be non-secure")
	}
}

func TestToggleThemeCookieUsesSecureFlagOnForwardedHTTPS(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/settings/toggle-theme", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	h.handleToggleTheme(rr, req)

	cookie := findResponseCookie(rr.Result().Cookies(), "theme")
	if cookie == nil {
		t.Fatal("expected theme cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected forwarded https theme cookie to be secure")
	}
}

func TestClearSessionCookiePreservesSameSite(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager()
	defer sm.Stop()

	req := httptest.NewRequest(http.MethodPost, "https://example.test/logout", nil)
	rr := httptest.NewRecorder()
	sm.ClearCookie(rr, req)

	cookie := findResponseCookie(rr.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatal("expected cleared session cookie")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
	}
	if !cookie.Secure {
		t.Fatal("expected cleared https session cookie to remain secure")
	}
}

func TestHandleExportIdentityUsesSessionIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	holder := identity.NewHolder(filepath.Join(dir, "identities"))
	firstKP, _, err := holder.CreateAndSave("passphrase")
	if err != nil {
		t.Fatalf("CreateAndSave(first) error = %v", err)
	}
	secondKP, _, err := holder.CreateAndSave("passphrase")
	if err != nil {
		t.Fatalf("CreateAndSave(second) error = %v", err)
	}

	sm := NewSessionManager()
	defer sm.Stop()
	token, err := sm.Create(firstKP)
	if err != nil {
		t.Fatalf("Create(session) error = %v", err)
	}

	h := &Handler{
		identity: holder,
		sessions: sm,
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.test/settings/export", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	h.handleExportIdentity(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var payload struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal export payload: %v", err)
	}
	if payload.Pubkey != sm.Get(token).PubkeyHex {
		t.Fatalf("export pubkey = %q, want session pubkey %q", payload.Pubkey, sm.Get(token).PubkeyHex)
	}
	if payload.Pubkey == hex.EncodeToString(secondKP.PublicKeyBytes()) {
		t.Fatalf("exported active identity %q instead of session identity", payload.Pubkey)
	}
}
