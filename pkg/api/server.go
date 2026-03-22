package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xleaks/xleaks/pkg/api/middleware"
)

// Server wraps the HTTP server, WebSocket hub, and all handler dependencies.
type Server struct {
	httpServer *http.Server
	wsHub      *WSHub
	router     http.Handler
}

// NewServer creates a new API server.
func NewServer(listenAddr string, deps *HandlerDeps) *Server {
	wsHub := NewWSHub()
	router := NewRouter(deps, wsHub)

	handler := middleware.LocalOnly(
		middleware.CORS(
			router,
		),
	)

	s := &Server{
		httpServer: &http.Server{
			Addr:         listenAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		wsHub:  wsHub,
		router: router,
	}

	return s
}

// Start begins listening for HTTP connections.
func (s *Server) Start() error {
	log.Printf("API server listening on %s", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// WSHub returns the WebSocket hub for broadcasting events.
func (s *Server) WSHub() *WSHub {
	return s.wsHub
}
