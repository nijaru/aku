package aku

import (
	"net/http"
	"slices"
)

// Group represents a group of routes with a common prefix and middleware.
type Group struct {
	app        *App
	prefix     string
	middleware []func(http.Handler) http.Handler
}

// Group creates a new sub-group from this group.
func (g *Group) Group(prefix string, mw ...func(http.Handler) http.Handler) *Group {
	return &Group{
		app:        g.app,
		prefix:     g.prefix + prefix,
		middleware: append(slices.Clone(g.middleware), mw...),
	}
}

func (g *Group) Handle(method, pattern string, handler http.Handler, route *Route) error {
	return g.app.Handle(method, g.prefix+pattern, handler, route)
}

// HandleHTTP registers a standard http.Handler on the group's prefix.
// Registration errors are returned to the caller.
func (g *Group) HandleHTTP(
	method, pattern string,
	handler http.Handler,
	opts ...RouteOption,
) error {
	return g.app.handleHTTP(method, g.prefix+pattern, handler, g.middleware, opts...)
}

// Metrics registers a standard http.Handler for serving metrics on the group's prefix.
// Registration errors are returned to the caller.
func (g *Group) Metrics(pattern string, handler http.Handler, opts ...RouteOption) error {
	return g.HandleHTTP(http.MethodGet, pattern, handler, opts...)
}

func (g *Group) App() *App                                     { return g.app }
func (g *Group) Prefix() string                                { return g.prefix }
func (g *Group) Middleware() []func(http.Handler) http.Handler { return g.middleware }
