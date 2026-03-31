package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServerWithConfigSetsHTTPTimeoutsAndHeaderLimit(t *testing.T) {
	t.Parallel()

	server := NewServerWithConfig(ServerConfig{
		ListenAddr:      "127.0.0.1:7470",
		EnableWebSocket: true,
	}, &HandlerDeps{})

	if server.httpServer == nil {
		t.Fatal("expected httpServer to be initialized")
	}
	if got := server.httpServer.Addr; got != "127.0.0.1:7470" {
		t.Fatalf("Addr = %q, want %q", got, "127.0.0.1:7470")
	}
	if got := server.httpServer.ReadHeaderTimeout; got != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", got, 5*time.Second)
	}
	if got := server.httpServer.ReadTimeout; got != 15*time.Second {
		t.Fatalf("ReadTimeout = %s, want %s", got, 15*time.Second)
	}
	if got := server.httpServer.WriteTimeout; got != 15*time.Second {
		t.Fatalf("WriteTimeout = %s, want %s", got, 15*time.Second)
	}
	if got := server.httpServer.IdleTimeout; got != 60*time.Second {
		t.Fatalf("IdleTimeout = %s, want %s", got, 60*time.Second)
	}
	if got := server.httpServer.MaxHeaderBytes; got != maxHTTPHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", got, maxHTTPHeaderBytes)
	}
	if server.httpServer.Handler == nil {
		t.Fatal("expected handler to be set")
	}
}

func TestNewServerWithConfigExposesWrappedHandler(t *testing.T) {
	t.Parallel()

	server := NewServerWithConfig(ServerConfig{
		ListenAddr:      "127.0.0.1:7470",
		EnableWebSocket: false,
	}, &HandlerDeps{})

	handler := server.Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
}
