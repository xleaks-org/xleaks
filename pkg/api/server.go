package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/xleaks/xleaks/pkg/api/middleware"
)

// ServerConfig holds configuration options for the API server.
type ServerConfig struct {
	ListenAddr string
	APIToken   string // empty = no auth required
}

// Server wraps the HTTP server, WebSocket hub, and all handler dependencies.
type Server struct {
	httpServer *http.Server
	wsHub      *WSHub
	router     http.Handler
	apiToken   string
}

// NewServer creates a new API server. It accepts a listen address string and
// handler dependencies. To configure an API token, use NewServerWithConfig.
func NewServer(listenAddr string, deps *HandlerDeps) *Server {
	return NewServerWithConfig(ServerConfig{ListenAddr: listenAddr}, deps)
}

// NewServerWithConfig creates a new API server from the given config.
func NewServerWithConfig(cfg ServerConfig, deps *HandlerDeps) *Server {
	wsHub := NewWSHub()
	router := NewRouter(deps, wsHub)

	// Build the middleware chain: LocalOnly -> optional TokenAuth -> CORS -> router.
	var handler http.Handler = router
	handler = middleware.CORS(handler)

	if cfg.APIToken != "" {
		handler = middleware.TokenAuth(cfg.APIToken)(handler)
	}

	handler = middleware.LocalOnly(handler)

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		wsHub:    wsHub,
		router:   router,
		apiToken: cfg.APIToken,
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

// GetToken returns the API token configured for this server. An empty string
// means token auth is not enabled.
func (s *Server) GetToken() string {
	return s.apiToken
}
