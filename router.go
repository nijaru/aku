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

// Router is the interface implemented by App and Group for route registration.
type Router interface {
	Handle(method, pattern string, handler http.Handler, route *Route)
	App() *App
	Prefix() string
	Middleware() []func(http.Handler) http.Handler
	Static(prefix, root string)
	StaticFS(prefix string, fs http.FileSystem)
}

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

func (g *Group) App() *App                            { return g.app }
func (g *Group) Prefix() string                       { return g.prefix }
func (g *Group) Middleware() []func(http.Handler) http.Handler { return g.middleware }

func (g *Group) Static(prefix, root string) {
	g.app.Static(g.prefix+prefix, root)
}

func (g *Group) StaticFS(prefix string, fs http.FileSystem) {
	g.app.StaticFS(g.prefix+prefix, fs)
}

// Get registers a new GET route on the router.
func Get[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodGet, pattern, handler, opts...)
}

// Post registers a new POST route on the router.
func Post[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodPost, pattern, handler, opts...)
}

// Put registers a new PUT route on the router.
func Put[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodPut, pattern, handler, opts...)
}

// Patch registers a new PATCH route on the router.
func Patch[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodPatch, pattern, handler, opts...)
}

// Delete registers a new DELETE route on the router.
func Delete[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodDelete, pattern, handler, opts...)
}

// Options registers a new OPTIONS route on the router.
func Options[In any, Out any](r Router, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	return register(r, http.MethodOptions, pattern, handler, opts...)
}

// register registers a new route with the router.
func register[In any, Out any](r Router, method, pattern string, handler Handler[In, Out], opts ...RouteOption) error {
	app := r.App()
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

	// Type to cache the reflect.Value and pointer together.
	type PooledIn struct {
		ptr *In
		val reflect.Value
	}

	// Pool for input structs to minimize allocations.
	pool := sync.Pool{
		New: func() any {
			ptr := new(In)
			return &PooledIn{
				ptr: ptr,
				val: reflect.ValueOf(ptr).Elem(),
			}
		},
	}

	// Define the wrapper handler.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pooled := pool.Get().(*PooledIn)
		in := pooled.ptr
		defer func() {
			// Zero out the struct before putting it back.
			var zero In
			*in = zero
			pool.Put(pooled)
		}()

		// 1. Extract and bind parameters.
		if err := extractor(r.Context(), r, in, pooled.val, app.bindConfig); err != nil {
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

	// Apply route-local middleware, then group middleware.
	var finalHandler http.Handler = h
	// Route-local first (innermost)
	for i := len(meta.middleware) - 1; i >= 0; i-- {
		finalHandler = meta.middleware[i](finalHandler)
	}
	// Group middleware (outer)
	groupMW := r.Middleware()
	for i := len(groupMW) - 1; i >= 0; i-- {
		finalHandler = groupMW[i](finalHandler)
	}

	fullPattern := r.Prefix() + pattern
	route := &Route{
		Method:      method,
		Pattern:     fullPattern,
		Status:      meta.status,
		Summary:     meta.summary,
		Description: meta.description,
		Tags:        meta.tags,
		Security:    meta.security,
		Schema:      schema,
		OutputType:  outType,
		middleware:  append(append([]func(http.Handler) http.Handler{}, groupMW...), meta.middleware...),
	}

	// Register with the router.
	r.Handle(method, pattern, finalHandler, route)

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
