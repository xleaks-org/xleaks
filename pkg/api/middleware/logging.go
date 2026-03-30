package middleware

import (
	"log/slog"
	"net/http"
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
