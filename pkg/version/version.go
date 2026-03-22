package version

var (
	// Version is the semantic version of the build, set via -ldflags at compile time.
	Version = "dev"

	// BuildTime is the UTC timestamp of the build, set via -ldflags at compile time.
	BuildTime = "unknown"
)
