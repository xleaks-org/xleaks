package middleware

import "net/http"

// NoStore prevents browsers and intermediary caches from storing sensitive responses.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Cache-Control", "no-store, max-age=0")
		headers.Set("Pragma", "no-cache")
		headers.Set("Expires", "0")

		next.ServeHTTP(w, r)
	})
}
