package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
