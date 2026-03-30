package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/config"
)

func TestSetupDatabaseCreatesOwnerOnlyDirectories(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Node.DataDir = t.TempDir()

	db, _, err := setupDatabase(cfg)
	if err != nil {
		t.Fatalf("setupDatabase() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, dir := range []string{
		cfg.DataDir(),
		filepath.Join(cfg.DataDir(), "identity"),
		filepath.Join(cfg.DataDir(), "identity", "keys"),
		filepath.Join(cfg.DataDir(), "data"),
		filepath.Join(cfg.DataDir(), "data", "objects"),
		filepath.Join(cfg.DataDir(), "data", "media"),
		filepath.Join(cfg.DataDir(), "logs"),
		filepath.Join(cfg.DataDir(), "cache"),
		filepath.Join(cfg.DataDir(), "cache", "thumbnails"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%s) error = %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("directory %s permissions = %o, want 700", dir, perm)
		}
	}
}
