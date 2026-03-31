package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func logAccessRejection(r *http.Request, reason string, attrs ...any) {
	base := []any{
		"reason", reason,
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
	}
	base = append(base, attrs...)
	slog.Warn("rejected api request", base...)
}

// RequestLogger logs API requests without including raw query strings.
func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(recorder, r)

			status := recorder.Status()
			if status == 0 {
				status = http.StatusOK
			}
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", recorder.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			}
			if requestID := chiMiddleware.GetReqID(r.Context()); requestID != "" {
				attrs = append(attrs, "request_id", requestID)
			}
			if r.URL.RawQuery != "" {
				attrs = append(attrs, "query_redacted", true)
			}

			slog.Info("api request", attrs...)
		})
	}
}
