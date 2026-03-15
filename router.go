package aku

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/internal/render"
)

// Handler is the canonical typed handler signature.
type Handler[In any, Out any] func(context.Context, In) (Out, error)

// RouteOption configures a specific route at registration time.
type RouteOption func(*routeMeta)

type routeMeta struct {
	status      int
	summary     string
	description string
	tags        []string
	security    []map[string][]string
	middleware  []func(http.Handler) http.Handler
	schema      *bind.Schema
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

// WithSummary sets a summary for the route, used in OpenAPI generation.
func WithSummary(summary string) RouteOption {
	return func(m *routeMeta) {
		m.summary = summary
	}
}

// WithDescription sets a detailed description for the route, used in OpenAPI generation.
func WithDescription(description string) RouteOption {
	return func(m *routeMeta) {
		m.description = description
	}
}

// WithTags adds tags to the route, used for grouping in OpenAPI generation.
func WithTags(tags ...string) RouteOption {
	return func(m *routeMeta) {
		m.tags = append(m.tags, tags...)
	}
}

// WithTag adds a single tag to the route.
func WithTag(tag string) RouteOption {
	return func(m *routeMeta) {
		m.tags = append(m.tags, tag)
	}
}

// WithSecurity adds security requirements to the route.
// Example: WithSecurity(map[string][]string{"BearerAuth": {}})
func WithSecurity(security ...map[string][]string) RouteOption {
	return func(m *routeMeta) {
		m.security = append(m.security, security...)
	}
}

// WithSecurityName adds a simple security requirement by name with no scopes.
func WithSecurityName(name string) RouteOption {
	return func(m *routeMeta) {
		m.security = append(m.security, map[string][]string{name: {}})
	}
}

// Get registers a new GET route on the application.
func Get[In any, Out any](app *App, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(app, http.MethodGet, pattern, handler, opts...)
}

// Post registers a new POST route on the application.
func Post[In any, Out any](app *App, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(app, http.MethodPost, pattern, handler, opts...)
}

// register registers a new route with the application.
func register[In any, Out any](app *App, method, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	meta := defaultRouteMeta()

	// Compile the extractor and schema once at startup.
	extractor, schema := bind.Compiler[In]()
	meta.schema = schema

	for _, opt := range opts {
		opt(&meta)
	}

	// Pre-determine response type for optimization.
	outType := reflect.TypeOf((*Out)(nil)).Elem()
	isReader := outType.Implements(reflect.TypeOf((*io.Reader)(nil)).Elem())
	isStream := outType == reflect.TypeOf(Stream{})
	isSSE := outType == reflect.TypeOf(SSE{})

	// Pool for input structs to minimize allocations.
	pool := sync.Pool{
		New: func() any {
			return new(In)
		},
	}

	// Define the wrapper handler.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in := pool.Get().(*In)
		defer func() {
			// Zero out the struct before putting it back.
			var zero In
			*in = zero
			pool.Put(in)
		}()

		// 1. Extract and bind parameters.
		if err := extractor(r.Context(), r, in, app.bindConfig); err != nil {
			if bindErr, ok := errors.AsType[*bind.BindError](err); ok {
				handleError(app, w, r, ValidationProblem("Request extraction or validation failed", []InvalidParam{
					{
						Name:   bindErr.Field,
						In:     bindErr.Source,
						Reason: bindErr.Err.Error(),
					},
				}))
			} else {
				if prob, ok := errors.AsType[*Problem](err); ok {
					handleError(app, w, r, prob)
				} else {
					handleError(app, w, r, BadRequest(err.Error()))
				}
			}
			return
		}

		// 2. Run validator if present.
		if app.validator != nil {
			if err := app.validator.Struct(in); err != nil {
				if vErr, ok := errors.AsType[validator.ValidationErrors](err); ok {
					handleError(app, w, r, ValidationProblem("Input validation failed", FromValidationErrors(vErr)))
				} else {
					handleError(app, w, r, BadRequest(err.Error()))
				}
				return
			}
		}

		// 3. Call the user handler.
		out, err := handler(r.Context(), *in)
		if err != nil {
			handleError(app, w, r, err)
			return
		}

		// 4. Render success response.
		if meta.status == http.StatusNoContent {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Handle streaming and special types
		if isReader {
			render.Reader(w, meta.status, any(out).(io.Reader), "application/octet-stream")
			return
		}
		if isStream {
			s := any(out).(Stream)
			render.Reader(w, meta.status, s.Reader, s.ContentType)
			return
		}
		if isSSE {
			sse := any(out).(SSE)
			events := make(chan render.SSEEvent)
			go func() {
				defer close(events)
				for {
					select {
					case <-r.Context().Done():
						return
					case e, ok := <-sse.Events:
						if !ok {
							return
						}
						select {
						case <-r.Context().Done():
							return
						case events <- render.SSEEvent{
							ID:    e.ID,
							Event: e.Event,
							Data:  e.Data,
						}:
						}
					}
				}
			}()
			render.SSE(w, r, events)
			return
		}

		render.JSON(w, meta.status, out)
	})

	// Apply route-local middleware.
	var finalHandler http.Handler = h
	for i := len(meta.middleware) - 1; i >= 0; i-- {
		finalHandler = meta.middleware[i](finalHandler)
	}

	// Register the handler with the mux.
	app.mux.Handle(method+" "+pattern, finalHandler)

	// Store route metadata for OpenAPI generation.
	app.routes = append(app.routes, &Route{
		Method:      method,
		Pattern:     pattern,
		Status:      meta.status,
		Summary:     meta.summary,
		Description: meta.description,
		Tags:        meta.tags,
		Security:    meta.security,
		Schema:      schema,
		OutputType:  outType,
		middleware:  meta.middleware,
	})

	return nil
}

func handleError(app *App, w http.ResponseWriter, r *http.Request, err error) {
	if app.errorHandler != nil {
		app.errorHandler(w, r, err)
		return
	}

	if prob, ok := errors.AsType[*Problem](err); ok {
		render.Problem(w, prob.Status, prob)
	} else {
		// Default behavior for non-Problem errors
		render.Problem(w, http.StatusInternalServerError, Problemf(http.StatusInternalServerError, "Internal Server Error", "%s", err.Error()))
	}
}
