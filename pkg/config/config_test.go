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
