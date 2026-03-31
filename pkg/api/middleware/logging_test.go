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
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

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
}
