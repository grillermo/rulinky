// Package web embeds the Vite build (web/dist) into the binary.
// Run `pnpm run build` before `go build`.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func Dist() (fs.FS, error) { return fs.Sub(dist, "dist") }
