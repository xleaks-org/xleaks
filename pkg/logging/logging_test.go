package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupCreatesOwnerOnlyLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "xleaks.log")

	if err := Setup("info", path); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", perm)
	}
}

func TestRedactPathPreservesOnlyBasename(t *testing.T) {
	value := RedactPath(filepath.Join("private", "nested", "config.toml"))

	if value.Kind() != slog.KindGroup {
		t.Fatalf("Kind = %v, want %v", value.Kind(), slog.KindGroup)
	}

	attrs := value.Group()
	if len(attrs) != 2 {
		t.Fatalf("group len = %d, want %d", len(attrs), 2)
	}
	if attrs[0].Key != "redacted" || !attrs[0].Value.Bool() {
		t.Fatalf("redacted attr = %v, want true", attrs[0])
	}
	if attrs[1].Key != "base" || attrs[1].Value.String() != "config.toml" {
		t.Fatalf("base attr = %v, want config.toml", attrs[1])
	}
}

func TestSetupTightensExistingLogFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xleaks.log")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Setup("info", path); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log file permissions = %o, want 600", perm)
	}
}
