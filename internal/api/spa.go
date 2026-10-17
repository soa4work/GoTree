package api

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

func init() {
	// Not in Go's built-in table; browsers want the proper type before they
	// offer to install the app.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// appCSP is the Content-Security-Policy of the app shell. Scripts only come
// from the app itself (the Vite build and theme.js are external files), which
// makes an injected inline script or handler inert. Styles stay 'unsafe-inline'
// because React style props and Leaflet position elements with inline styles.
// Images may also be OpenStreetMap tiles (the migration map) and data: or
// blob: URLs (the wall chart's SVG download embeds photos as data URIs).
const appCSP = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https://tile.openstreetmap.org; " +
	"font-src 'self' data:; " +
	"media-src 'self' blob:; " +
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	"worker-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// spaHandler serves files from the built frontend and falls back to
// index.html for client-side routes.
func spaHandler(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(dist, name); err == nil && !info.IsDir() {
				// Vite puts content-hashed files under assets/; everything
				// else (manifest, icons) may change without a new name.
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
			// A missing file with an extension is a broken asset reference,
			// not a client-side route; answering with index.html would hand
			// the browser HTML where it expects a script or an image.
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}

		index, err := fs.ReadFile(dist, "index.html")
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "The frontend is not built. Run `make build` (or `npm run build` in web/) and restart.", http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			http.Error(w, "cannot read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", appCSP)
		w.Header().Set("Cache-Control", "no-cache")
		// A share link's address is its password; search engines must not
		// list pages that someone posted it on.
		if strings.HasPrefix(r.URL.Path, "/share/") {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
		_, _ = w.Write(index)
	})
}
