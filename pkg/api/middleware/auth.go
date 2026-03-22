package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
)

// LocalOnly restricts API access to requests originating from localhost.
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "Forbidden: API only accessible from localhost", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
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
