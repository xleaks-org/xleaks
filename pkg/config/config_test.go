package config

import (
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
