package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
)

// LocalOnly restricts API access to requests originating from localhost. When
// allowForwarded is false, proxy-forwarding headers are rejected so loopback
// binds cannot be exposed through a reverse proxy without token auth.
func LocalOnly(allowForwarded bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				logAccessRejection(r, "invalid_remote_addr")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				logAccessRejection(r, "non_loopback_remote", "remote_host", host)
				http.Error(w, "Forbidden: API only accessible from localhost", http.StatusForbidden)
				return
			}
			if !allowForwarded {
				if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
					logAccessRejection(r, "proxy_forwarding_not_allowed", "header", "X-Forwarded-For")
					http.Error(w, "Forbidden: proxied API access requires token auth", http.StatusForbidden)
					return
				}
				if forwarded := strings.TrimSpace(r.Header.Get("Forwarded")); forwarded != "" {
					logAccessRejection(r, "proxy_forwarding_not_allowed", "header", "Forwarded")
					http.Error(w, "Forbidden: proxied API access requires token auth", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TokenAuth middleware validates Bearer token from requests. If the provided
// token is empty, auth is disabled and all requests are allowed through.
func TokenAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no token configured, skip auth entirely.
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for OPTIONS (CORS preflight).
			if r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header.
			auth := r.Header.Get("Authorization")
			if auth == "Bearer "+token {
				next.ServeHTTP(w, r)
				return
			}

			// Also check query parameter for WebSocket connections.
			if r.URL.Query().Get("token") == token {
				next.ServeHTTP(w, r)
				return
			}

			logAccessRejection(r, "invalid_api_token", "has_authorization", auth != "", "has_query_token", r.URL.Query().Has("token"))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

// GenerateToken creates a random 32-byte hex token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SaveToken writes the token to a file with mode 0600 (owner-only read/write).
func SaveToken(token, path string) error {
	return os.WriteFile(path, []byte(token), 0600)
}

// LoadToken reads the token from a file and trims whitespace.
func LoadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
