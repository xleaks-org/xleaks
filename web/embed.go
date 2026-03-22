package web

import "embed"

// StaticFiles embeds the Next.js build output for serving from the Go binary.
// The .next/static directory contains compiled JS/CSS chunks.
// In production, the Go binary serves these as static assets and proxies
// dynamic routes to the Next.js standalone server bundled alongside.
//
//go:embed public
var StaticFiles embed.FS
