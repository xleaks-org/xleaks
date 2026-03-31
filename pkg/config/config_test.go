package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDefaultConfigIncludesKnownIndexers(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if len(cfg.Indexer.KnownIndexers) == 0 {
		t.Fatal("expected built-in known indexers")
	}
	if !slices.Equal(cfg.Indexer.KnownIndexers, DefaultKnownIndexers()) {
		t.Fatalf("expected default config known indexers %v, got %v", DefaultKnownIndexers(), cfg.Indexer.KnownIndexers)
	}
	if !cfg.API.EnableWebUI {
		t.Fatal("expected web UI to be enabled by default")
	}
	if cfg.API.AllowRemoteWebUI {
		t.Fatal("expected remote web UI exposure to be disabled by default")
	}
}

func TestMaxStorageBytes(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if got := cfg.MaxStorageBytes(); got != 5*1024*1024*1024 {
		t.Fatalf("default MaxStorageBytes = %d, want %d", got, int64(5*1024*1024*1024))
	}

	cfg.Node.MaxStorageGB = 0
	if got := cfg.MaxStorageBytes(); got != 0 {
		t.Fatalf("zero MaxStorageBytes = %d, want 0", got)
	}

	cfg.Node.MaxStorageGB = -3
	if got := cfg.MaxStorageBytes(); got != 0 {
		t.Fatalf("negative MaxStorageBytes = %d, want 0", got)
	}
}

func TestValidateStorageLimit(t *testing.T) {
	t.Parallel()

	t.Run("user mode requires minimum", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		cfg.Node.MaxStorageGB = 0
		if err := cfg.ValidateStorageLimit(); err == nil {
			t.Fatal("expected validation error for zero user-mode storage limit")
		}
	})

	t.Run("indexer mode allows zero", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		cfg.Node.Mode = "indexer"
		cfg.Node.MaxStorageGB = 0
		if err := cfg.ValidateStorageLimit(); err != nil {
			t.Fatalf("ValidateStorageLimit() error = %v", err)
		}
	})

	t.Run("negative is always rejected", func(t *testing.T) {
		t.Parallel()

		if err := ValidateStorageLimitForMode("indexer", -1); err == nil {
			t.Fatal("expected validation error for negative storage limit")
		}
	})
}

func TestSaveCreatesOwnerOnlyConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := DefaultConfig()
	cfg.Node.MaxStorageGB = 9

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config permissions = %o, want 600", perm)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Node.MaxStorageGB != 9 {
		t.Fatalf("saved MaxStorageGB = %d, want 9", loaded.Node.MaxStorageGB)
	}
}

func TestSaveCreatesOwnerOnlyConfigDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, "nested", "config")
	path := filepath.Join(configDir, "config.toml")
	cfg := DefaultConfig()

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat config dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config dir permissions = %o, want 700", perm)
	}
}

func TestSaveTightensExistingConfigDirectoryPermissions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(configDir, 0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	path := filepath.Join(configDir, "config.toml")
	cfg := DefaultConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat config dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config dir permissions = %o, want 700", perm)
	}
}

func TestSavePreservesExistingConfigFileMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[node]\nmax_storage_gb = 1\n"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Node.MaxStorageGB = 12
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("config permissions = %o, want 640", perm)
	}
}

func TestSavePreservesSymlinkPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.toml")
	link := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, []byte("[node]\nmax_storage_gb = 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink not supported: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Node.MaxStorageGB = 14
	if err := cfg.Save(link); err != nil {
		t.Fatalf("Save via symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected config path to remain a symlink")
	}

	loaded, err := Load(target)
	if err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if loaded.Node.MaxStorageGB != 14 {
		t.Fatalf("saved MaxStorageGB = %d, want 14", loaded.Node.MaxStorageGB)
	}
}
