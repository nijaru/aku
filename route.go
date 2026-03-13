package aku

import "net/http"

// Get registers a new GET route on the application.
func Get[In any, Out any](app *App, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(app, http.MethodGet, pattern, handler, opts...)
}

// Post registers a new POST route on the application.
func Post[In any, Out any](app *App, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(app, http.MethodPost, pattern, handler, opts...)
}

func register[In any, Out any](app *App, method, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	meta := defaultRouteMeta()
	for _, opt := range opts {
		opt(&meta)
	}

	// For scaffolding MVP: The internal planning logic and actual handler invocation
	// are not yet wired up. We register a placeholder.
	app.mux.HandleFunc(method+" "+pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})

	return nil
}
