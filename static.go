package aku

import (
	"io"
	"net/http"
	"path"
	"strings"
)

// Static registers a directory of static files to be served at the given prefix.
func (a *App) Static(prefix, root string, opts ...RouteOption) {
	a.StaticFS(prefix, http.Dir(root), opts...)
}

// StaticFS registers a file system of static files to be served at the given prefix.
func (a *App) StaticFS(prefix string, fs http.FileSystem, opts ...RouteOption) {
	meta := defaultRouteMeta()
	for _, opt := range opts {
		opt(&meta)
	}

	// Go 1.22 mux routing requires matching exact paths or directories with trailing slashes.
	pattern := prefix
	if !strings.HasSuffix(pattern, "/") {
		a.mux.Handle("GET "+pattern, http.RedirectHandler(pattern+"/", http.StatusMovedPermanently))
		pattern += "/"
	}

	fileServer := http.FileServer(fs)
	stripped := http.StripPrefix(strings.TrimSuffix(prefix, "/"), fileServer)

	var handler http.Handler = stripped
	if meta.spa {
		handler = &spaHandler{
			fs:   fs,
			next: stripped,
		}
	}

	// Apply middleware.
	for i := len(meta.middleware) - 1; i >= 0; i-- {
		handler = meta.middleware[i](handler)
	}

	// We use a.mux.Handle directly here to support subtree matching (trailing slash).
	a.mux.Handle("GET "+pattern, handler)

	// Add to routes for OpenAPI (optional, usually static files are internal)
	if !meta.internal {
		a.routes = append(a.routes, &Route{
			Method:      "GET",
			Pattern:     prefix + "*",
			Summary:     meta.summary,
			Description: meta.description,
			Internal:    true, // Static files are internal by default
		})
	}
}

func (g *Group) Static(prefix, root string, opts ...RouteOption) {
	g.app.Static(g.prefix+prefix, root, opts...)
}

func (g *Group) StaticFS(prefix string, fs http.FileSystem, opts ...RouteOption) {
	g.app.StaticFS(g.prefix+prefix, fs, opts...)
}

type spaHandler struct {
	fs   http.FileSystem
	next http.Handler
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sw := &spaResponseWriter{ResponseWriter: w}
	h.next.ServeHTTP(sw, r)

	if sw.status == http.StatusNotFound {
		// If it's a request for a file with an extension (e.g. .js, .css, .png),
		// we don't want to fallback to index.html as it would serve HTML with the wrong content-type.
		if path.Ext(r.URL.Path) != "" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 page not found\n"))
			return
		}

		// Fallback to index.html
		f, err := h.fs.Open("index.html")
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 page not found (and no index.html)\n"))
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Use ServeContent to handle range requests, caching, etc.
		http.ServeContent(w, r, "index.html", stat.ModTime(), f.(io.ReadSeeker))
	}
}

type spaResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *spaResponseWriter) WriteHeader(status int) {
	w.status = status
	if status != http.StatusNotFound {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *spaResponseWriter) Write(b []byte) (int, error) {
	if w.status == http.StatusNotFound {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
