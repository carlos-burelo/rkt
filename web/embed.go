// Package web embebe la SPA compilada (web/dist, build pnpm).
// Sin build: solo .gitkeep -> dashboard responde 503 con pista.
package web

import "embed"

// Dist SPA compilada. CI corre `pnpm --dir web build` antes de go build.
//
//go:embed dist
var Dist embed.FS
