package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewIndexerHTTPServerSetsTimeouts(t *testing.T) {
	t.Parallel()

	srv := newIndexerHTTPServer("127.0.0.1:8081", http.NotFoundHandler())

	if srv.Addr != "127.0.0.1:8081" {
		t.Fatalf("Addr = %q, want %q", srv.Addr, "127.0.0.1:8081")
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, 5*time.Second)
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %s, want %s", srv.ReadTimeout, 15*time.Second)
	}
	if srv.WriteTimeout != 15*time.Second {
		t.Fatalf("WriteTimeout = %s, want %s", srv.WriteTimeout, 15*time.Second)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %s, want %s", srv.IdleTimeout, 60*time.Second)
	}
	if srv.Handler == nil {
		t.Fatal("expected handler to be set")
	}
}
