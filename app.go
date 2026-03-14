package aku

import "net/http"

// Option configures an App instance.
type Option func(*App)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mux        *http.ServeMux
	middleware []func(http.Handler) http.Handler
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

// Use adds global middleware to the application.
func (a *App) Use(mw ...func(http.Handler) http.Handler) {
	a.middleware = append(a.middleware, mw...)
}

// ServeHTTP implements the standard library http.Handler interface.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var finalHandler http.Handler = a.mux
	for i := len(a.middleware) - 1; i >= 0; i-- {
		finalHandler = a.middleware[i](finalHandler)
	}
	finalHandler.ServeHTTP(w, r)
}
