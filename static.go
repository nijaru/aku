package aku

import (
	"net/http"
	"strings"
)

func (a *App) Static(prefix, root string) {
	a.StaticFS(prefix, http.Dir(root))
}

func (a *App) StaticFS(prefix string, fs http.FileSystem) {
	// Go 1.22 mux routing requires matching exact paths or directories with trailing slashes.
	if !strings.HasSuffix(prefix, "/") {
		// Register the exact prefix to redirect to the trailing slash version,
		// or serve the index if it exists.
		exactPrefix := prefix
		prefix += "/"
		a.mux.Handle(exactPrefix, http.RedirectHandler(prefix, http.StatusMovedPermanently))
	}
	handler := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(fs))
	a.mux.Handle(prefix, handler)
}

func (g *Group) Static(prefix, root string) {
	g.app.Static(g.prefix+prefix, root)
}

func (g *Group) StaticFS(prefix string, fs http.FileSystem) {
	g.app.StaticFS(g.prefix+prefix, fs)
}
