package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/xleaks-org/xleaks/pkg/api/middleware"
	"github.com/xleaks-org/xleaks/pkg/metrics"
	"github.com/xleaks-org/xleaks/pkg/version"
)

// serverStartTime records when the server package was first loaded, used
// to compute the uptime reported by the /health endpoint.
var serverStartTime = time.Now()

// ServerConfig holds configuration options for the API server.
type ServerConfig struct {
	ListenAddr      string
	APIToken        string // empty = no auth required
	EnableWebSocket bool
}

// Server wraps the HTTP server, WebSocket hub, and all handler dependencies.
type Server struct {
	httpServer *http.Server
	wsHub      *WSHub
	wsTickets  *WSTicketManager
	router     http.Handler
	apiToken   string
}

// NewServer creates a new API server. It accepts a listen address string and
// handler dependencies. To configure an API token, use NewServerWithConfig.
func NewServer(listenAddr string, deps *HandlerDeps) *Server {
	return NewServerWithConfig(ServerConfig{ListenAddr: listenAddr, EnableWebSocket: true}, deps)
}

// NewServerWithConfig creates a new API server from the given config.
func NewServerWithConfig(cfg ServerConfig, deps *HandlerDeps) *Server {
	var wsHub *WSHub
	var wsTickets *WSTicketManager
	if cfg.EnableWebSocket {
		wsHub = NewWSHub()
		wsTickets = NewWSTicketManager(defaultWSTicketTTL)
	}
	if deps == nil {
		deps = &HandlerDeps{}
	}
	deps.WSTickets = wsTickets
	router := NewRouter(deps, wsHub)

	// Build the middleware chain (outermost first, innermost last):
	//   CORS (outermost) -> TokenAuth (optional) -> BrowserGuard -> LocalOnly -> router
	var handler http.Handler = router
	handler = middleware.LocalOnly(cfg.APIToken != "")(handler)
	handler = middleware.BrowserGuard(handler)

	if cfg.APIToken != "" {
		handler = middleware.TokenAuthWithWebSocketTicket(cfg.APIToken, wsTickets.ValidateAndConsume)(handler)
	}

	handler = middleware.CORS()(handler)

	metricsHandler := middleware.LocalOnly(cfg.APIToken != "")(metrics.Handler())
	if cfg.APIToken != "" {
		metricsHandler = middleware.TokenAuth(cfg.APIToken)(metricsHandler)
	}

	// Create a top-level mux so /health bypasses all auth/local middleware while
	// /metrics still follows the server's local/token exposure policy.
	topMux := http.NewServeMux()
	topMux.HandleFunc("GET /health", handleHealth)
	topMux.Handle("GET /metrics", metricsHandler)
	topMux.Handle("/", handler)

	serverHandler := middleware.SecurityHeaders(topMux)

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.ListenAddr,
			Handler:      serverHandler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		wsHub:     wsHub,
		wsTickets: wsTickets,
		router:    router,
		apiToken:  cfg.APIToken,
	}

	return s
}

// handleHealth responds with the node's health status, version, and uptime.
// This endpoint is unauthenticated and bypasses the main auth/local middleware.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "ok",
		"version":        version.Version,
		"uptime_seconds": int(time.Since(serverStartTime).Seconds()),
	})
}

// Start begins listening for HTTP connections.
func (s *Server) Start() error {
	slog.Info("API server listening", "addr", s.httpServer.Addr)
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

// Handler returns the fully wrapped HTTP handler used by the server.
func (s *Server) Handler() http.Handler {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Handler
}

// GetToken returns the API token configured for this server. An empty string
// means token auth is not enabled.
func (s *Server) GetToken() string {
	return s.apiToken
}
