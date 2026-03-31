package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
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
	httpServer  *http.Server
	wsHub       *WSHub
	wsTickets   *WSTicketManager
	browserAuth *BrowserAuthManager
	router      http.Handler
	apiToken    string
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
	var browserAuth *BrowserAuthManager
	if cfg.EnableWebSocket {
		wsHub = NewWSHub()
		wsTickets = NewWSTicketManager(defaultWSTicketTTL)
	}
	if cfg.APIToken != "" {
		browserAuth = NewBrowserAuthManager(defaultBrowserAuthTTL)
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
		handler = browserSessionAuth(cfg.APIToken, browserAuth)(handler)
	}

	handler = middleware.CORS()(handler)

	metricsHandler := middleware.LocalOnly(cfg.APIToken != "")(metrics.Handler())
	if cfg.APIToken != "" {
		metricsHandler = middleware.TokenAuth(cfg.APIToken)(metricsHandler)
	}

	// Create a top-level mux so /health bypasses all auth/local middleware while
	// /metrics still follows the server's local/token exposure policy.
	topMux := http.NewServeMux()
	if browserAuth != nil {
		authLimiter := middleware.NewRouteRateLimiter()
		authLimiter.AddLimit("POST /auth/token", 5, time.Minute)
		authLimiter.SetGlobalLimit(30, time.Minute)

		topMux.Handle("GET /auth/token", middleware.NoStore(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextPath := safeBrowserRedirectPath(r.URL.Query().Get("next"))
			state, _ := browserAuth.CookieState(r)
			if state == browserAuthSessionValid {
				http.Redirect(w, r, nextPath, http.StatusSeeOther)
				return
			}
			errorCode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("error")))
			if state == browserAuthSessionStale {
				browserAuth.ClearCookie(w, r)
				if errorCode == "" {
					errorCode = "expired"
				}
			}
			renderBrowserAuthPage(w, nextPath, errorCode)
		})))
		loginHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			nextPath := safeBrowserRedirectPath(r.FormValue("next"))
			if !authTokensEqual(r.FormValue("token"), cfg.APIToken) {
				logBrowserAuthWarn(r, "browser auth login failed", "reason", "invalid_token", "next", nextPath)
				http.Redirect(w, r, "/auth/token?error=invalid&next="+url.QueryEscape(nextPath), http.StatusSeeOther)
				return
			}

			sessionToken, err := browserAuth.Issue()
			if err != nil {
				slog.Error("browser auth session issue failed", "path", r.URL.Path, "remote_addr", r.RemoteAddr, "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			browserAuth.SetCookie(w, r, sessionToken)
			logBrowserAuthInfo(r, "browser auth login succeeded", "next", nextPath)
			http.Redirect(w, r, nextPath, http.StatusSeeOther)
		})
		topMux.Handle("POST /auth/token", middleware.NoStore(observeStatus(authLimiter.Middleware(loginHandler), func(r *http.Request, status int) {
			if status == http.StatusTooManyRequests {
				logBrowserAuthWarn(r, "browser auth login rate limited")
			}
		})))
	}
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
		wsHub:       wsHub,
		wsTickets:   wsTickets,
		browserAuth: browserAuth,
		router:      router,
		apiToken:    cfg.APIToken,
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
	err := s.httpServer.Shutdown(ctx)
	if s.wsHub != nil {
		s.wsHub.Close()
	}
	return err
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

func browserSessionAuth(apiToken string, browserAuth *BrowserAuthManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiToken == "" || browserAuth == nil || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			sessionState, sessionToken := browserAuth.CookieState(r)

			if r.Header.Get("Authorization") == "Bearer "+apiToken {
				if shouldBootstrapBrowserSession(r) && sessionState != browserAuthSessionValid {
					if token, err := browserAuth.Issue(); err == nil {
						browserAuth.SetCookie(w, r, token)
					} else {
						slog.Error("browser auth bootstrap failed", "path", r.URL.Path, "remote_addr", r.RemoteAddr, "error", err)
					}
				}
				next.ServeHTTP(w, r)
				return
			}

			if sessionState == browserAuthSessionValid {
				clone := r.Clone(r.Context())
				clone.Header = r.Header.Clone()
				clone.Header.Set("Authorization", "Bearer "+apiToken)
				if r.Method == http.MethodPost && r.URL.Path == "/logout" {
					buffered := newBufferedResponseWriter()
					next.ServeHTTP(buffered, clone)
					if buffered.StatusCode() < http.StatusBadRequest {
						browserAuth.Revoke(sessionToken)
						browserAuth.ClearCookie(buffered, r)
						if buffered.StatusCode() == http.StatusSeeOther {
							buffered.Header().Set("Location", "/auth/token?error=logged_out&next=%2F")
						}
						logBrowserAuthInfo(r, "browser auth session revoked", "reason", "logout")
					}
					buffered.FlushTo(w)
					return
				}
				next.ServeHTTP(w, clone)
				return
			}

			if sessionState == browserAuthSessionStale {
				nextPath := safeBrowserRedirectPath(r.URL.RequestURI())
				browserAuth.ClearCookie(w, r)
				if shouldBootstrapBrowserSession(r) {
					logBrowserAuthInfo(r, "browser auth session expired", "mode", "navigation", "next", nextPath)
					http.Redirect(w, r, "/auth/token?error=expired&next="+url.QueryEscape(nextPath), http.StatusSeeOther)
					return
				}
				w.Header().Set("Cache-Control", "no-store, max-age=0")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
				w.Header().Set(browserAuthExpiredHeader, browserAuthExpiredValue)
				logBrowserAuthInfo(r, "browser auth session expired", "mode", "background", "next", nextPath)
				http.Error(w, "Browser access expired", http.StatusUnauthorized)
				return
			}

			if shouldBootstrapBrowserSession(r) {
				http.Redirect(w, r, "/auth/token?next="+url.QueryEscape(safeBrowserRedirectPath(r.URL.RequestURI())), http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

func (r *statusRecorder) StatusCode() int {
	if !r.wroteHeader {
		return http.StatusOK
	}
	return r.status
}

func observeStatus(next http.Handler, after func(*http.Request, int)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if after != nil {
			after(r, recorder.StatusCode())
		}
	})
}

type bufferedResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

func (w *bufferedResponseWriter) StatusCode() int {
	return w.status
}

func (w *bufferedResponseWriter) FlushTo(dst http.ResponseWriter) {
	for key, values := range w.header {
		dst.Header()[key] = append([]string(nil), values...)
	}
	dst.WriteHeader(w.status)
	_, _ = dst.Write(w.body.Bytes())
}

func logBrowserAuthInfo(r *http.Request, message string, attrs ...any) {
	slog.Info(message, browserAuthLogAttrs(r, attrs...)...)
}

func logBrowserAuthWarn(r *http.Request, message string, attrs ...any) {
	slog.Warn(message, browserAuthLogAttrs(r, attrs...)...)
}

func browserAuthLogAttrs(r *http.Request, attrs ...any) []any {
	base := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
	}
	base = append(base, attrs...)
	return base
}
