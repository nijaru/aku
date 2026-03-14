package aku

import (
	"context"
	"net/http"

	"github.com/nijaru/aku/internal/bind"
)

// Handler is the canonical typed handler signature.
type Handler[In any, Out any] func(context.Context, In) (Out, error)

// RouteOption configures a specific route at registration time.
type RouteOption func(*routeMeta)

type routeMeta struct {
	status     int
	middleware []func(http.Handler) http.Handler
	schema     *bind.Schema
}

func defaultRouteMeta() routeMeta {
	return routeMeta{
		status: http.StatusOK,
	}
}

// WithStatus overrides the success HTTP status code for a route.
func WithStatus(code int) RouteOption {
	return func(m *routeMeta) {
		m.status = code
	}
}

// WithMiddleware adds route-local middleware to the handler.
func WithMiddleware(mw ...func(http.Handler) http.Handler) RouteOption {
	return func(m *routeMeta) {
		m.middleware = append(m.middleware, mw...)
	}
}
