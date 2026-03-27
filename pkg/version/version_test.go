package version

import (
	"testing"
)

func TestDefaultVersion(t *testing.T) {
	t.Parallel()
	if Version == "" {
		t.Error("Version should have a default value, got empty string")
	}
	if Version != "dev" {
		t.Errorf("Version default = %q, want %q", Version, "dev")
	}
}

func TestDefaultBuildTime(t *testing.T) {
	t.Parallel()
	if BuildTime == "" {
		t.Error("BuildTime should have a default value, got empty string")
	}
	if BuildTime != "unknown" {
		t.Errorf("BuildTime default = %q, want %q", BuildTime, "unknown")
	}
}

func TestVersionIsOverridable(t *testing.T) {
	// Save originals.
	origVersion := Version
	origBuild := BuildTime
	t.Cleanup(func() {
		Version = origVersion
		BuildTime = origBuild
	})

	Version = "v1.2.3"
	BuildTime = "2025-01-01T00:00:00Z"

	if Version != "v1.2.3" {
		t.Errorf("Version = %q after override, want v1.2.3", Version)
	}
	if BuildTime != "2025-01-01T00:00:00Z" {
		t.Errorf("BuildTime = %q after override, want 2025-01-01T00:00:00Z", BuildTime)
	}
}
