package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	return TokenAuthWithWebSocketTicket(token, nil)
}

// TokenAuthWithWebSocketTicket validates Bearer tokens and optionally accepts
// one-time WebSocket tickets on upgrade requests.
func TokenAuthWithWebSocketTicket(token string, validateWebSocketTicket func(string) bool) func(http.Handler) http.Handler {
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

			// Allow query-string tokens only on actual WebSocket handshakes.
			if isWebSocketHandshake(r) {
				if validateWebSocketTicket != nil && validateWebSocketTicket(r.URL.Query().Get("ws_ticket")) {
					next.ServeHTTP(w, r)
					return
				}
				if r.URL.Query().Get("token") == token {
					next.ServeHTTP(w, r)
					return
				}
			}

			logAccessRejection(r, "invalid_api_token",
				"has_authorization", auth != "",
				"has_query_token", r.URL.Query().Has("token"),
				"has_ws_ticket", r.URL.Query().Has("ws_ticket"),
			)
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
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	if err := syncParentDirectory(dir); err != nil {
		return fmt.Errorf("sync token directory parent: %w", err)
	}

	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp token file: %w", err)
	}
	tempPath := f.Name()
	defer os.Remove(tempPath)

	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return fmt.Errorf("set token permissions: %w", err)
	}
	if _, err := f.Write([]byte(token)); err != nil {
		f.Close()
		return fmt.Errorf("write token: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync token: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	if err := syncParentDirectory(dir); err != nil {
		return fmt.Errorf("sync token directory: %w", err)
	}
	return nil
}

// LoadToken reads the token from a file and trims whitespace.
func LoadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func syncParentDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer dir.Close()

	if err := dir.Sync(); err != nil {
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) {
			return nil
		}
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}

func isWebSocketHandshake(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			return true
		}
	}
	return false
}
