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

func TestBrowserAuthManagerEvictsOldestSessionWhenAtCapacity(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewBrowserAuthManager(time.Minute)
	m.now = func() time.Time { return now }
	m.maxCount = 2

	first, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() first error = %v", err)
	}
	now = now.Add(time.Second)
	second, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() second error = %v", err)
	}
	now = now.Add(time.Second)
	third, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() third error = %v", err)
	}

	if m.Validate(first) {
		t.Fatal("expected oldest browser auth session to be evicted")
	}
	if !m.Validate(second) {
		t.Fatal("expected newer browser auth session to remain valid")
	}
	if !m.Validate(third) {
		t.Fatal("expected newest browser auth session to remain valid")
	}
	if got := len(m.sessions); got != 2 {
		t.Fatalf("session count = %d, want %d", got, 2)
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

func TestShouldBootstrapBrowserSessionRejectsHTMXRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/web/node-status", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("HX-Request", "true")

	if shouldBootstrapBrowserSession(req) {
		t.Fatal("expected htmx request to skip browser-session bootstrap redirects")
	}
}

func TestBrowserSessionAuthRedirectsExpiredHTMLRequestToReauthAndClearsCookie(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewBrowserAuthManager(time.Minute)
	m.now = func() time.Time { return now }

	token, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	now = now.Add(2 * time.Minute)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/signup", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: browserAuthCookieName, Value: token})

	rr := httptest.NewRecorder()
	called := false

	browserSessionAuth("secret", m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if called {
		t.Fatal("expired browser auth request should not reach handler")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if got := rr.Header().Get("Location"); got != "/auth/token?error=expired&next=%2Fsignup" {
		t.Fatalf("Location = %q, want %q", got, "/auth/token?error=expired&next=%2Fsignup")
	}

	cookie := findCookie(rr.Result().Cookies(), browserAuthCookieName)
	if cookie == nil {
		t.Fatal("expected cleared browser auth cookie")
	}
	if cookie.MaxAge != -1 {
		t.Fatalf("MaxAge = %d, want %d", cookie.MaxAge, -1)
	}
}

func TestBrowserSessionAuthReturnsExpiredHeaderForHTMXRequest(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	m := NewBrowserAuthManager(time.Minute)
	m.now = func() time.Time { return now }

	token, err := m.Issue()
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	now = now.Add(2 * time.Minute)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/web/node-status", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: browserAuthCookieName, Value: token})

	rr := httptest.NewRecorder()
	called := false

	browserSessionAuth("secret", m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if called {
		t.Fatal("expired browser auth htmx request should not reach handler")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if got := rr.Header().Get(browserAuthExpiredHeader); got != browserAuthExpiredValue {
		t.Fatalf("expired header = %q, want %q", got, browserAuthExpiredValue)
	}
	if location := rr.Header().Get("Location"); location != "" {
		t.Fatalf("Location = %q, want empty", location)
	}

	cookie := findCookie(rr.Result().Cookies(), browserAuthCookieName)
	if cookie == nil {
		t.Fatal("expected cleared browser auth cookie")
	}
	if cookie.MaxAge != -1 {
		t.Fatalf("MaxAge = %d, want %d", cookie.MaxAge, -1)
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
