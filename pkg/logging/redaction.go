package logging

import (
	"log/slog"
	"path/filepath"
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
