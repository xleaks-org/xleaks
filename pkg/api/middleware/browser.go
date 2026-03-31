package middleware

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	xlog "github.com/xleaks-org/xleaks/pkg/logging"
)

const (
	browserCSRFCookieName = "xleaks_csrf"
	browserCSRFHeaderName = "X-CSRF-Token"
	browserCSRFTokenLen   = 32
)

// BrowserGuard rejects cross-origin browser requests and requires the shared
// CSRF token on unsafe browser-originated API calls.
func BrowserGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site == "cross-site" {
			logAccessRejection(r, "cross_site_browser_request", "sec_fetch_site", site)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !OriginAllowed(r, origin) {
			logAccessRejection(r, "cross_origin_request", "origin", xlog.RedactURL(origin))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" && !sameOriginURL(r, referer) {
			logAccessRejection(r, "cross_origin_referer", "referer", xlog.RedactURL(referer))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if requiresBrowserCSRF(r) {
			cookie, err := r.Cookie(browserCSRFCookieName)
			if err != nil || !validBrowserCSRFToken(cookie.Value) {
				logAccessRejection(r, "missing_browser_csrf_cookie")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			token := strings.TrimSpace(r.Header.Get(browserCSRFHeaderName))
			if !validBrowserCSRFToken(token) || subtle.ConstantTimeCompare([]byte(token), []byte(cookie.Value)) != 1 {
				logAccessRejection(r, "invalid_browser_csrf", "has_header_token", token != "")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// OriginAllowed reports whether the supplied origin matches the current request origin.
func OriginAllowed(r *http.Request, origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return sameOrigin(r, u.Scheme, u.Host)
}

func requiresBrowserCSRF(r *http.Request) bool {
	if r == nil || !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	if isSafeMethod(r.Method) {
		return false
	}
	return r.Header.Get("Origin") != "" ||
		r.Header.Get("Referer") != "" ||
		r.Header.Get("Sec-Fetch-Site") != "" ||
		r.Header.Get("Cookie") != ""
}

func sameOriginURL(r *http.Request, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return sameOrigin(r, u.Scheme, u.Host)
}

func sameOrigin(r *http.Request, scheme, host string) bool {
	return strings.EqualFold(requestScheme(r), scheme) && strings.EqualFold(requestHost(r), host)
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := forwardedScheme(r); scheme != "" {
		return scheme
	}
	return "http"
}

func requestHost(r *http.Request) string {
	if host := forwardedHost(r); host != "" {
		return host
	}
	return r.Host
}

func forwardedScheme(r *http.Request) string {
	if r == nil {
		return ""
	}
	if scheme := parseForwardedPair(r.Header.Get("Forwarded"), "proto"); scheme != "" {
		return scheme
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		return ""
	}
	if comma := strings.IndexByte(scheme, ','); comma >= 0 {
		scheme = scheme[:comma]
	}
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	switch scheme {
	case "http", "https":
		return scheme
	default:
		return ""
	}
}

func forwardedHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host := parseForwardedPair(r.Header.Get("Forwarded"), "host"); host != "" {
		return host
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		return ""
	}
	if comma := strings.IndexByte(host, ','); comma >= 0 {
		host = host[:comma]
	}
	return strings.TrimSpace(host)
}

func parseForwardedPair(forwarded, key string) string {
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

func validBrowserCSRFToken(token string) bool {
	if len(token) != browserCSRFTokenLen*2 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
