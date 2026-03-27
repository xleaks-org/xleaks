package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"
)

var (
	startTime        = time.Now()
	requestsTotal    atomic.Int64
	errorsTotal      atomic.Int64
	postsCreated     atomic.Int64
	activeWebSockets atomic.Int64
)

// IncrRequests increments the total HTTP request counter.
func IncrRequests() { requestsTotal.Add(1) }

// IncrErrors increments the total error counter.
func IncrErrors() { errorsTotal.Add(1) }

// IncrPosts increments the posts-created counter.
func IncrPosts() { postsCreated.Add(1) }

// IncrWS increments the active WebSocket connections gauge.
func IncrWS() { activeWebSockets.Add(1) }

// DecrWS decrements the active WebSocket connections gauge.
func DecrWS() { activeWebSockets.Add(-1) }

// Handler returns an http.HandlerFunc that emits Prometheus-compatible
// text/plain metrics. No external library required.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintf(w, "# HELP xleaks_uptime_seconds Time since server start in seconds.\n")
		fmt.Fprintf(w, "# TYPE xleaks_uptime_seconds gauge\n")
		fmt.Fprintf(w, "xleaks_uptime_seconds %f\n\n", time.Since(startTime).Seconds())

		fmt.Fprintf(w, "# HELP xleaks_requests_total Total number of HTTP requests served.\n")
		fmt.Fprintf(w, "# TYPE xleaks_requests_total counter\n")
		fmt.Fprintf(w, "xleaks_requests_total %d\n\n", requestsTotal.Load())

		fmt.Fprintf(w, "# HELP xleaks_errors_total Total number of HTTP error responses (4xx/5xx).\n")
		fmt.Fprintf(w, "# TYPE xleaks_errors_total counter\n")
		fmt.Fprintf(w, "xleaks_errors_total %d\n\n", errorsTotal.Load())

		fmt.Fprintf(w, "# HELP xleaks_posts_created_total Total number of posts created.\n")
		fmt.Fprintf(w, "# TYPE xleaks_posts_created_total counter\n")
		fmt.Fprintf(w, "xleaks_posts_created_total %d\n\n", postsCreated.Load())

		fmt.Fprintf(w, "# HELP xleaks_websocket_connections_active Number of active WebSocket connections.\n")
		fmt.Fprintf(w, "# TYPE xleaks_websocket_connections_active gauge\n")
		fmt.Fprintf(w, "xleaks_websocket_connections_active %d\n\n", activeWebSockets.Load())

		fmt.Fprintf(w, "# HELP xleaks_go_goroutines Number of goroutines.\n")
		fmt.Fprintf(w, "# TYPE xleaks_go_goroutines gauge\n")
		fmt.Fprintf(w, "xleaks_go_goroutines %d\n\n", runtime.NumGoroutine())

		fmt.Fprintf(w, "# HELP xleaks_go_memstats_alloc_bytes Number of bytes allocated and in use.\n")
		fmt.Fprintf(w, "# TYPE xleaks_go_memstats_alloc_bytes gauge\n")
		fmt.Fprintf(w, "xleaks_go_memstats_alloc_bytes %d\n\n", m.Alloc)

		fmt.Fprintf(w, "# HELP xleaks_go_memstats_sys_bytes Number of bytes obtained from system.\n")
		fmt.Fprintf(w, "# TYPE xleaks_go_memstats_sys_bytes gauge\n")
		fmt.Fprintf(w, "xleaks_go_memstats_sys_bytes %d\n\n", m.Sys)

		fmt.Fprintf(w, "# HELP xleaks_go_gc_completed_total Number of completed GC cycles.\n")
		fmt.Fprintf(w, "# TYPE xleaks_go_gc_completed_total counter\n")
		fmt.Fprintf(w, "xleaks_go_gc_completed_total %d\n", m.NumGC)
	}
}
