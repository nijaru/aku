package aku

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strings"
)

// Static registers a directory of static files to be served at the given prefix.
func (a *App) Static(prefix, root string, opts ...RouteOption) {
	a.staticFS(prefix, http.Dir(root), nil, opts...)
}

// StaticFS registers a file system of static files to be served at the given prefix.
func (a *App) StaticFS(prefix string, fs http.FileSystem, opts ...RouteOption) {
	a.staticFS(prefix, fs, nil, opts...)
}

func (a *App) staticFS(
	prefix string,
	fs http.FileSystem,
	parentMiddleware []func(http.Handler) http.Handler,
	opts ...RouteOption,
) {
	meta := defaultRouteMeta()
	for _, opt := range opts {
		opt(&meta)
	}

	// Go 1.22 mux routing requires matching exact paths or directories with trailing slashes.
	pattern := prefix
	if !strings.HasSuffix(pattern, "/") {
		redirect := http.RedirectHandler(pattern+"/", http.StatusMovedPermanently)
		redirect = wrapHandler(redirect, parentMiddleware)
		a.registerHandler("GET "+pattern, redirect)
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
	handler = wrapHandler(handler, meta.middleware)
	handler = wrapHandler(handler, parentMiddleware)

	// We use a.mux.Handle directly here to support subtree matching (trailing slash).
	a.registerHandler("GET "+pattern, handler)
}

func (g *Group) Static(prefix, root string, opts ...RouteOption) {
	g.app.staticFS(g.prefix+prefix, http.Dir(root), g.middleware, opts...)
}

func (g *Group) StaticFS(prefix string, fs http.FileSystem, opts ...RouteOption) {
	g.app.staticFS(g.prefix+prefix, fs, g.middleware, opts...)
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
		f, err := h.fs.Open("/index.html")
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

		// FileServer writes a text/plain content type before reporting its 404.
		// Clear that suppressed error metadata so the fallback is served as HTML.
		w.Header().Del("Content-Type")
		w.Header().Del("Content-Length")

		// Use ServeContent to handle range requests and caching when the file is
		// seekable. Custom http.FileSystem implementations are allowed to return
		// non-seekable files, so buffer those rather than panicking on a type cast.
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
			return
		}
		data, err := io.ReadAll(f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, "index.html", stat.ModTime(), bytes.NewReader(data))
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

func (w *spaResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
