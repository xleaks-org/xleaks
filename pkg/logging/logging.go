package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Setup initialises the global slog logger based on configuration values.
// It writes JSON-structured log lines to stderr and, when filePath is non-empty,
// also to the specified file (creating parent directories as needed).
func Setup(level, filePath string) error {
	var w io.Writer = os.Stderr

	if filePath != "" {
		filePath = expandHome(filePath)
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			return err
		}
		w = io.MultiWriter(os.Stderr, f)
	}

	lvl := parseLevel(level)
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
	return nil
}

// parseLevel converts a human-readable level string to slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// expandHome expands a leading "~/" to the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
