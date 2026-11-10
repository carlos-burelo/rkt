// Package web embebe la SPA compilada (web/dist, build pnpm).
// go:embed exige al menos un archivo no-oculto en dist: el build es
// obligatorio (CI corre `pnpm --dir web build` antes de go build).
// En runtime, sin index.html el dashboard responde 503 con pista.
package web

import "embed"

// Dist SPA compilada. CI corre `pnpm --dir web build` antes de go build.
//
//go:embed dist
var Dist embed.FS
