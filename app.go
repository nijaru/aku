package aku

import "net/http"

// Option configures an App instance.
type Option func(*App)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mux *http.ServeMux
}

// New creates a new Aku application.
func New(opts ...Option) *App {
	a := &App{
		mux: http.NewServeMux(),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// ServeHTTP implements the standard library http.Handler interface.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}
