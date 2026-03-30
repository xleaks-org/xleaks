package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	browserAuthCookieName    = "xleaks_browser_auth"
	defaultBrowserAuthTTL    = 24 * time.Hour
	browserAuthExpiredHeader = "X-XLeaks-Browser-Auth"
	browserAuthExpiredValue  = "expired"
)

type browserAuthSessionState uint8

const (
	browserAuthSessionMissing browserAuthSessionState = iota
	browserAuthSessionValid
	browserAuthSessionStale
)

type BrowserAuthManager struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	ttl      time.Duration
	now      func() time.Time
}

func NewBrowserAuthManager(ttl time.Duration) *BrowserAuthManager {
	if ttl <= 0 {
		ttl = defaultBrowserAuthTTL
	}
	return &BrowserAuthManager{
		sessions: make(map[string]time.Time),
		ttl:      ttl,
		now:      time.Now,
	}
}

func (m *BrowserAuthManager) Issue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	token := hex.EncodeToString(b)
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)
	m.sessions[token] = now.Add(m.ttl)
	return token, nil
}

func (m *BrowserAuthManager) Validate(token string) bool {
	return m.sessionState(token) == browserAuthSessionValid
}

func (m *BrowserAuthManager) ValidateRequest(r *http.Request) bool {
	state, _ := m.CookieState(r)
	return state == browserAuthSessionValid
}

func (m *BrowserAuthManager) CookieState(r *http.Request) (browserAuthSessionState, string) {
	if m == nil || r == nil {
		return browserAuthSessionMissing, ""
	}
	cookie, err := r.Cookie(browserAuthCookieName)
	if err != nil || cookie.Value == "" {
		return browserAuthSessionMissing, ""
	}
	return m.sessionState(cookie.Value), cookie.Value
}

func (m *BrowserAuthManager) Revoke(token string) bool {
	if m == nil || token == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.sessions[token]
	delete(m.sessions, token)
	return ok
}

func (m *BrowserAuthManager) SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     browserAuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(m.ttl.Seconds()),
		HttpOnly: true,
		Secure:   browserAuthRequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *BrowserAuthManager) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     browserAuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   browserAuthRequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *BrowserAuthManager) cleanupExpiredLocked(now time.Time) {
	for token, expiresAt := range m.sessions {
		if !now.Before(expiresAt) {
			delete(m.sessions, token)
		}
	}
}

func (m *BrowserAuthManager) sessionState(token string) browserAuthSessionState {
	if m == nil || token == "" {
		return browserAuthSessionMissing
	}

	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupExpiredLocked(now)

	expiresAt, ok := m.sessions[token]
	if !ok || !now.Before(expiresAt) {
		delete(m.sessions, token)
		return browserAuthSessionStale
	}

	m.sessions[token] = now.Add(m.ttl)
	return browserAuthSessionValid
}

func browserAuthRequestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if proto := browserAuthForwardedProto(r); proto != "" {
		return strings.EqualFold(proto, "https")
	}
	return false
}

func browserAuthForwardedProto(r *http.Request) string {
	if r == nil {
		return ""
	}
	if proto := browserAuthForwardedPair(r.Header.Get("Forwarded"), "proto"); proto != "" {
		return proto
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		return ""
	}
	if comma := strings.IndexByte(proto, ','); comma >= 0 {
		proto = proto[:comma]
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	switch proto {
	case "http", "https":
		return proto
	default:
		return ""
	}
}

func browserAuthForwardedPair(forwarded, key string) string {
	for _, part := range strings.Split(forwarded, ",") {
		for _, field := range strings.Split(part, ";") {
			name, value, ok := strings.Cut(field, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), key) {
				continue
			}
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"`)
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func authTokensEqual(actual, expected string) bool {
	if len(actual) != len(expected) || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func isBrowserHTMLRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Requested-With")), "XMLHttpRequest") {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		return false
	}
	if mode := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode"))); mode == "navigate" {
		return true
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	return strings.Contains(accept, "text/html")
}

func shouldBootstrapBrowserSession(r *http.Request) bool {
	if !isBrowserHTMLRequest(r) {
		return false
	}
	switch r.URL.Path {
	case "/health", "/metrics":
		return false
	}
	return !strings.HasPrefix(r.URL.Path, "/api/")
}

func safeBrowserRedirectPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "/"
	}
	if u.Path == "/auth/token" {
		return "/"
	}
	return u.RequestURI()
}

func renderBrowserAuthPage(w http.ResponseWriter, nextPath, errorCode string) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var errorHTML string
	switch strings.ToLower(strings.TrimSpace(errorCode)) {
	case "invalid":
		errorHTML = `<p style="color:#ef4444;margin:0 0 16px 0;">Invalid access token.</p>`
	case "expired":
		errorHTML = `<p style="color:#f59e0b;margin:0 0 16px 0;">Browser access expired. Enter the access token again.</p>`
	case "logged_out":
		errorHTML = `<p style="color:#10b981;margin:0 0 16px 0;">Browser access locked. Enter the access token again to continue.</p>`
	}

	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>XLeaks Access</title>
</head>
<body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0a0a0a;color:#ededed;font-family:system-ui,-apple-system,sans-serif;">
  <main style="width:min(420px,calc(100vw - 32px));padding:32px;border:1px solid #1f1f1f;border-radius:16px;background:#111;">
    <h1 style="margin:0 0 12px 0;font-size:28px;">Access Required</h1>
    <p style="margin:0 0 20px 0;color:#9ca3af;">Enter the node access token to unlock the built-in web UI.</p>
    ` + errorHTML + `
    <form method="POST" action="/auth/token">
      <input type="hidden" name="next" value="` + template.HTMLEscapeString(nextPath) + `">
      <label for="token" style="display:block;margin-bottom:8px;font-size:14px;color:#d1d5db;">Access Token</label>
      <input id="token" name="token" type="password" autocomplete="current-password" required
        style="width:100%;box-sizing:border-box;padding:12px 14px;border-radius:10px;border:1px solid #333;background:#1a1a1a;color:#fff;margin-bottom:16px;">
      <button type="submit"
        style="width:100%;padding:12px 14px;border:0;border-radius:10px;background:#10b981;color:#04130d;font-weight:700;cursor:pointer;">
        Continue
      </button>
    </form>
  </main>
</body>
</html>`))
}
