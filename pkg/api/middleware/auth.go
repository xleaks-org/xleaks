package middleware

import (
	"net"
	"net/http"
)

// LocalOnly restricts API access to requests originating from localhost.
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "Forbidden: API only accessible from localhost", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
