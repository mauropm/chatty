package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"

	"chatty/web"
)

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Asset cache-busting: index.html is served no-cache, so rewriting every
// /static/ URL with a content-derived build token guarantees browsers fetch
// fresh files the moment a new binary is deployed — no hard reload needed.
var (
	assetsOnce sync.Once
	assetsVer  string
	indexOnce  sync.Once
	indexOut   []byte
)

var assetURLRe = regexp.MustCompile(`(/static/[^"'()\s?#]+\.(?:css|js|mjs|svg|png|ico|woff2|woff|json|webmanifest))`)

func computeAssetsVersion() string {
	h := sha256.New()
	static := web.Static()
	_ = fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, err := fs.ReadFile(static, p); err == nil {
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	h.Write(web.Index())
	return hex.EncodeToString(h.Sum(nil)[:8])
}

func versionedIndex() []byte {
	indexOnce.Do(func() {
		assetsOnce.Do(func() { assetsVer = computeAssetsVersion() })
		// ReplaceAllString works on an immutable string copy — never mutate the
		// shared embedded index bytes (ReplaceAllFunc + append would alias them).
		indexOut = []byte(assetURLRe.ReplaceAllString(string(web.Index()), "${1}?v="+assetsVer))
	})
	return indexOut
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(versionedIndex())
}

// staticHandler serves embedded assets with correct content types and caching.
// Code assets (css/js/json/svg) use ETag revalidation so deploys are seen
// immediately; fonts and images keep a day of caching.
func (s *Server) staticHandler() http.Handler {
	static := web.Static()
	fs := http.FileServer(http.FS(static))
	contentTypes := map[string]string{
		".css":  "text/css; charset=utf-8",
		".js":   "text/javascript; charset=utf-8",
		".mjs":  "text/javascript; charset=utf-8",
		".json": "application/json; charset=utf-8",
		".svg":  "image/svg+xml",
		".woff2": "font/woff2",
		".woff": "font/woff",
		".png":  "image/png",
		".ico":  "image/x-icon",
		".webmanifest": "application/manifest+json",
	}
	longCacheExt := map[string]bool{".woff2": true, ".woff": true, ".png": true, ".ico": true}
	// Pre-hash embedded contents once at startup for strong validators.
	etags := map[string]string{}
	fsWalk(static, func(name string, b []byte) {
		sum := sha256.Sum256(b)
		etags[name] = `"` + hex.EncodeToString(sum[:12]) + `"`
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		ext := strings.ToLower(path.Ext(name))
		if ct, ok := contentTypes[ext]; ok {
			w.Header().Set("Content-Type", ct)
		}
		if etag, ok := etags[name]; ok {
			w.Header().Set("ETag", etag)
		}
		if longCacheExt[ext] {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}

// fsWalk reads every file in the embedded FS once (small, ~1 MB).
func fsWalk(static fs.FS, fn func(name string, b []byte)) {
	_ = fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, err := fs.ReadFile(static, p); err == nil {
			fn(p, b)
		}
		return nil
	})
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	// Serve the SVG mark for browsers that request /favicon.ico.
	http.ServeFileFS(w, r, web.Static(), "images/logo.svg")
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	manifest := map[string]any{
		"name":             "Chatty",
		"short_name":       "Chatty",
		"description":      "Your thoughts, amplified.",
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#101014",
		"theme_color":      "#101014",
		"icons": []map[string]any{
			{"src": "/static/images/logo.svg", "sizes": "any", "type": "image/svg+xml"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(manifest)
}
