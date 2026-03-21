package aku

import (
	"net/http"
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
		middleware: append(append([]func(http.Handler) http.Handler{}, g.middleware...), mw...),
	}
}

func (g *Group) Handle(method, pattern string, handler http.Handler, route *Route) {
	g.app.Handle(method, g.prefix+pattern, handler, route)
}

// HandleHTTP registers a standard http.Handler on the group's prefix.
func (g *Group) HandleHTTP(method, pattern string, handler http.Handler, opts ...RouteOption) {
	g.app.HandleHTTP(method, g.prefix+pattern, handler, opts...)
}

// Metrics registers a standard http.Handler for serving metrics on the group's prefix.
func (g *Group) Metrics(pattern string, handler http.Handler, opts ...RouteOption) {
	g.HandleHTTP(http.MethodGet, pattern, handler, opts...)
}

func (g *Group) App() *App                                     { return g.app }
func (g *Group) Prefix() string                                { return g.prefix }
func (g *Group) Middleware() []func(http.Handler) http.Handler { return g.middleware }

// WS satisfies the Router interface for WebSockets.
func (g *Group) WS(pattern string, handler any, opts ...RouteOption) error {
	panic("use aku.WS(router, pattern, handler) instead")
}
