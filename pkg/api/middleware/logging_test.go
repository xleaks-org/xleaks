package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRequestLoggerRedactsQueryStringValues(t *testing.T) {
	buf := captureDefaultJSONLogger(t)

	handler := chiMiddleware.RequestID(RequestLogger()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/search?q=private+query&token=secret", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	logLine := strings.TrimSpace(buf.String())
	if logLine == "" {
		t.Fatal("expected log output")
	}
	if strings.Contains(logLine, "private+query") || strings.Contains(logLine, "secret") {
		t.Fatalf("log line leaked raw query values: %s", logLine)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(logLine), &payload); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	if got := payload["msg"]; got != "api request" {
		t.Fatalf("msg = %v, want %q", got, "api request")
	}
	if got := payload["path"]; got != "/api/search" {
		t.Fatalf("path = %v, want %q", got, "/api/search")
	}
	if got := payload["status"]; got != float64(http.StatusCreated) {
		t.Fatalf("status = %v, want %d", got, http.StatusCreated)
	}
	if got := payload["bytes"]; got != float64(2) {
		t.Fatalf("bytes = %v, want %d", got, 2)
	}
	if got := payload["query_redacted"]; got != true {
		t.Fatalf("query_redacted = %v, want true", got)
	}
	if got := payload["request_id"]; got == "" {
		t.Fatal("expected request_id in log payload")
	}
	remoteAddr, ok := payload["remote_addr"].(map[string]any)
	if !ok {
		t.Fatalf("remote_addr = %T, want object", payload["remote_addr"])
	}
	if got := remoteAddr["scope"]; got != "loopback" {
		t.Fatalf("remote_addr.scope = %v, want %q", got, "loopback")
	}
	if got := remoteAddr["port"]; got != "12345" {
		t.Fatalf("remote_addr.port = %v, want %q", got, "12345")
	}
}

func TestBrowserGuardRejectLogRedactsOriginDetails(t *testing.T) {
	buf := captureDefaultJSONLogger(t)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/config", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("Origin", "https://evil.example/private?q=secret")
	rr := httptest.NewRecorder()

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("cross-origin request should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}

	logLine := strings.TrimSpace(buf.String())
	if strings.Contains(logLine, "evil.example") || strings.Contains(logLine, "secret") || strings.Contains(logLine, "private") {
		t.Fatalf("log line leaked raw origin details: %s", logLine)
	}

	payload := decodeSingleJSONLogLine(t, logLine)
	if got := payload["msg"]; got != "rejected api request" {
		t.Fatalf("msg = %v, want %q", got, "rejected api request")
	}
	if got := payload["reason"]; got != "cross_origin_request" {
		t.Fatalf("reason = %v, want %q", got, "cross_origin_request")
	}

	origin, ok := payload["origin"].(map[string]any)
	if !ok {
		t.Fatalf("origin = %T, want object", payload["origin"])
	}
	if got := origin["scheme"]; got != "https" {
		t.Fatalf("origin.scheme = %v, want %q", got, "https")
	}
	if got := origin["scope"]; got != "hostname" {
		t.Fatalf("origin.scope = %v, want %q", got, "hostname")
	}
	if got := origin["path_redacted"]; got != true {
		t.Fatalf("origin.path_redacted = %v, want true", got)
	}
	if got := origin["query_redacted"]; got != true {
		t.Fatalf("origin.query_redacted = %v, want true", got)
	}
}

func TestLocalOnlyRejectLogDoesNotLeakRemoteHost(t *testing.T) {
	buf := captureDefaultJSONLogger(t)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/status", nil)
	req.RemoteAddr = "203.0.113.5:4567"
	rr := httptest.NewRecorder()

	LocalOnly(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("non-loopback request should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}

	logLine := strings.TrimSpace(buf.String())
	if strings.Contains(logLine, "203.0.113.5") {
		t.Fatalf("log line leaked raw remote host: %s", logLine)
	}

	payload := decodeSingleJSONLogLine(t, logLine)
	if _, ok := payload["remote_host"]; ok {
		t.Fatalf("unexpected remote_host field in log payload: %v", payload["remote_host"])
	}

	remoteAddr, ok := payload["remote_addr"].(map[string]any)
	if !ok {
		t.Fatalf("remote_addr = %T, want object", payload["remote_addr"])
	}
	if got := remoteAddr["scope"]; got != "public" {
		t.Fatalf("remote_addr.scope = %v, want %q", got, "public")
	}
	if got := remoteAddr["port"]; got != "4567" {
		t.Fatalf("remote_addr.port = %v, want %q", got, "4567")
	}
}

func captureDefaultJSONLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}

func decodeSingleJSONLogLine(t *testing.T, logLine string) map[string]any {
	t.Helper()

	if logLine == "" {
		t.Fatal("expected log output")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(logLine), &payload); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	return payload
}
