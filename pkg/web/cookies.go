package web

import (
	"net/http"
	"strings"
)

func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if forwardedProto == "" {
		return false
	}
	if comma := strings.IndexByte(forwardedProto, ','); comma >= 0 {
		forwardedProto = forwardedProto[:comma]
	}
	return strings.EqualFold(strings.TrimSpace(forwardedProto), "https")
}
