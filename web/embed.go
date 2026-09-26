// Package web embeds the built SPA so the api binary serves the frontend
// from its own bytes. The real assets are produced by `make web-build`
// (Vite) into web/dist; the committed placeholder keeps `go build` working
// before the first build and is replaced wholesale by Vite output.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
