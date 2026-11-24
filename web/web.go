// Package web embeds Chatty's front-end assets (HTML, CSS, JS, fonts, icons)
// into the Go binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var staticFS embed.FS

//go:embed index.html
var indexHTML []byte

// Index returns the single-page application shell.
func Index() []byte { return indexHTML }

// Static returns the embedded static asset filesystem rooted at the static/
// directory (so assets live at /css, /js, /fonts, /images).
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("web: static fs: " + err.Error())
	}
	return sub
}
