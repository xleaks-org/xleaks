package web

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

func TestSessionCookieSecureFlagMatchesRequestScheme(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager()
	defer sm.Stop()

	httpReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	httpResp := httptest.NewRecorder()
	sm.SetCookie(httpResp, httpReq, "token-http")

	httpCookie := findResponseCookie(httpResp.Result().Cookies(), sessionCookieName)
	if httpCookie == nil {
		t.Fatal("expected http session cookie")
	}
	if httpCookie.Secure {
		t.Fatal("expected http session cookie to be non-secure")
	}

	httpsReq := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	httpsResp := httptest.NewRecorder()
	sm.SetCookie(httpsResp, httpsReq, "token-https")

	httpsCookie := findResponseCookie(httpsResp.Result().Cookies(), sessionCookieName)
	if httpsCookie == nil {
		t.Fatal("expected https session cookie")
	}
	if !httpsCookie.Secure {
		t.Fatal("expected https session cookie to be secure")
	}
}

func TestSessionCookieUsesSecureFlagOnForwardedHTTPS(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager()
	defer sm.Stop()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()
	sm.SetCookie(rr, req, "token-forwarded")

	cookie := findResponseCookie(rr.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatal("expected forwarded session cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected forwarded https session cookie to be secure")
	}
}

func TestSessionManagerRotateForRequestInvalidatesPreviousToken(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager()
	defer sm.Stop()

	oldKeyPair, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() old error = %v", err)
	}
	newKeyPair, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() new error = %v", err)
	}

	oldToken, err := sm.Create(oldKeyPair)
	if err != nil {
		t.Fatalf("Create() old session error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "https://example.test/settings/switch", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: oldToken})
	rr := httptest.NewRecorder()

	newToken, err := sm.RotateForRequest(rr, req, newKeyPair)
	if err != nil {
		t.Fatalf("RotateForRequest() error = %v", err)
	}
	if newToken == oldToken {
		t.Fatal("expected replacement session token to differ from old token")
	}
	if sm.Get(oldToken) != nil {
		t.Fatal("expected old session token to be destroyed")
	}

	newSession := sm.Get(newToken)
	if newSession == nil {
		t.Fatal("expected rotated session to exist")
	}
	if newSession.PubkeyHex != hex.EncodeToString(newKeyPair.PublicKeyBytes()) {
		t.Fatalf("PubkeyHex = %q, want %q", newSession.PubkeyHex, hex.EncodeToString(newKeyPair.PublicKeyBytes()))
	}

	cookie := findResponseCookie(rr.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatal("expected replacement session cookie")
	}
	if cookie.Value != newToken {
		t.Fatalf("cookie value = %q, want %q", cookie.Value, newToken)
	}
}
