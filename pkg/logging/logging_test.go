package logging

import (
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
