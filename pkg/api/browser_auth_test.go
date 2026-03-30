package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBrowserAuthManagerValidatesAndRefreshesCookieSession(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewBrowserAuthManager(time.Minute)
	m.now = func() time.Time { return now }

	token, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	now = now.Add(30 * time.Second)
	if !m.Validate(token) {
		t.Fatal("expected issued browser auth token to validate")
	}

	now = now.Add(45 * time.Second)
	if !m.Validate(token) {
		t.Fatal("expected browser auth token refresh to extend lifetime")
	}
}

func TestBrowserAuthManagerRejectsExpiredCookieSession(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewBrowserAuthManager(time.Minute)
	m.now = func() time.Time { return now }

	token, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	now = now.Add(2 * time.Minute)
	if m.Validate(token) {
		t.Fatal("expected expired browser auth token to be rejected")
	}
}

func TestBrowserAuthManagerSetsSecureCookieOnForwardedHTTPS(t *testing.T) {
	t.Parallel()

	m := NewBrowserAuthManager(time.Minute)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/auth/token", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	m.SetCookie(rr, req, "browser-session")

	cookie := findCookie(rr.Result().Cookies(), browserAuthCookieName)
	if cookie == nil {
		t.Fatal("expected browser auth cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected forwarded https browser auth cookie to be secure")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
