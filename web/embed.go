package web

import "embed"

// FS contains the static frontend served by the HTTP adapter.
//
//go:embed index.html static
var FS embed.FS
