package web

import "embed"

// StaticFiles embeds the public directory for serving static assets
// (images, icons, etc.) from the Go binary.
//
//go:embed public
var StaticFiles embed.FS
