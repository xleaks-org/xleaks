package logging

import (
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// RedactPath preserves only the basename of a filesystem path in logs.
func RedactPath(path string) slog.Value {
	if path == "" {
		return slog.GroupValue(slog.Bool("redacted", true))
	}

	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) {
		base = clean
	}

	return slog.GroupValue(
		slog.Bool("redacted", true),
		slog.String("base", base),
	)
}

// RedactAddr preserves only coarse classification and port information for a
// network address.
func RedactAddr(addr string) slog.Value {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return slog.GroupValue(slog.Bool("redacted", true))
	}

	if strings.HasPrefix(addr, "/") {
		return redactMultiaddr(addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		return slog.GroupValue(
			slog.Bool("redacted", true),
			slog.String("scope", classifyHost(host)),
			slog.String("port", port),
		)
	}

	return slog.GroupValue(
		slog.Bool("redacted", true),
		slog.String("scope", classifyHost(addr)),
	)
}

// RedactURL preserves only the scheme and coarse host classification for a URL.
func RedactURL(raw string) slog.Value {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return slog.GroupValue(slog.Bool("redacted", true))
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return slog.GroupValue(slog.Bool("redacted", true))
	}

	attrs := []slog.Attr{
		slog.Bool("redacted", true),
		slog.String("scheme", strings.ToLower(u.Scheme)),
	}
	for _, attr := range RedactAddr(u.Host).Group() {
		if attr.Key == "redacted" {
			continue
		}
		attrs = append(attrs, attr)
	}
	if u.Path != "" && u.Path != "/" {
		attrs = append(attrs, slog.Bool("path_redacted", true))
	}
	if u.RawQuery != "" {
		attrs = append(attrs, slog.Bool("query_redacted", true))
	}

	return slog.GroupValue(attrs...)
}

func redactMultiaddr(addr string) slog.Value {
	parts := strings.Split(strings.TrimPrefix(addr, "/"), "/")
	protocols := make([]string, 0, len(parts)/2)
	scope := "multiaddr"
	port := ""

	for i := 0; i < len(parts); i += 2 {
		proto := strings.TrimSpace(parts[i])
		if proto == "" {
			continue
		}
		protocols = append(protocols, proto)
		value := ""
		if i+1 < len(parts) {
			value = strings.TrimSpace(parts[i+1])
		}
		switch proto {
		case "tcp", "udp":
			if port == "" && value != "" {
				port = value
			}
		case "ip4", "ip6", "dns", "dns4", "dns6":
			if scope == "multiaddr" && value != "" {
				scope = classifyHost(value)
			}
		}
	}

	attrs := []slog.Attr{
		slog.Bool("redacted", true),
		slog.String("scope", scope),
	}
	if len(protocols) > 0 {
		attrs = append(attrs, slog.Any("protocols", protocols))
	}
	if port != "" {
		attrs = append(attrs, slog.String("port", port))
	}
	return slog.GroupValue(attrs...)
}

func classifyHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	switch {
	case host == "":
		return "unknown"
	case strings.EqualFold(host, "localhost"):
		return "loopback"
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "hostname"
	}
	if ip.IsLoopback() {
		return "loopback"
	}
	if ip.IsPrivate() {
		return "private"
	}
	return "public"
}
