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
	forwardedProto := forwardedProto(r)
	if forwardedProto == "" {
		return false
	}
	return strings.EqualFold(forwardedProto, "https")
}

func forwardedProto(r *http.Request) string {
	if r == nil {
		return ""
	}
	if proto := forwardedHeaderValue(r.Header.Get("Forwarded"), "proto"); proto != "" {
		return proto
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		return ""
	}
	if comma := strings.IndexByte(proto, ','); comma >= 0 {
		proto = proto[:comma]
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	switch proto {
	case "http", "https":
		return proto
	default:
		return ""
	}
}

func forwardedHeaderValue(forwarded, key string) string {
	for _, part := range strings.Split(forwarded, ",") {
		for _, field := range strings.Split(part, ";") {
			name, value, ok := strings.Cut(field, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), key) {
				continue
			}
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"`)
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				return value
			}
		}
	}
	return ""
}
