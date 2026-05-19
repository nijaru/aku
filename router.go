package aku

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/internal/render"
	"github.com/nijaru/aku/problem"
)

// Handler is the canonical typed handler signature.
type Handler[In any, Out any] func(context.Context, In) (Out, error)

// Router is the interface implemented by App and Group for route registration.
type Router interface {
	Handle(method, pattern string, handler http.Handler, route *Route)
	HandleHTTP(method, pattern string, handler http.Handler, opts ...RouteOption)
	Metrics(pattern string, handler http.Handler, opts ...RouteOption)
	App() *App
	Prefix() string
	Middleware() []func(http.Handler) http.Handler
	Static(prefix, root string, opts ...RouteOption)
	StaticFS(prefix string, fs http.FileSystem, opts ...RouteOption)
}

func wrapHandler(handler http.Handler, middleware []func(http.Handler) http.Handler) http.Handler {
	finalHandler := handler
	for i := len(middleware) - 1; i >= 0; i-- {
		finalHandler = middleware[i](finalHandler)
	}
	return finalHandler
}

// Get registers a new GET route on the router.
func Get[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodGet, pattern, handler, opts...)
}

// Post registers a new POST route on the router.
func Post[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodPost, pattern, handler, opts...)
}

// Put registers a new PUT route on the router.
func Put[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodPut, pattern, handler, opts...)
}

// Patch registers a new PATCH route on the router.
func Patch[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodPatch, pattern, handler, opts...)
}

// Delete registers a new DELETE route on the router.
func Delete[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodDelete, pattern, handler, opts...)
}

// Options registers a new OPTIONS route on the router.
func Options[In any, Out any](
	r Router,
	pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	return register(r, http.MethodOptions, pattern, handler, opts...)
}

// register registers a new route with the router.
func register[In any, Out any](
	r Router,
	method, pattern string,
	handler Handler[In, Out],
	opts ...RouteOption,
) error {
	app := r.App()
	meta := defaultRouteMeta()

	// Compile the extractor and schema once at startup.
	extractor, schema := bind.Compiler[In]()
	meta.schema = schema

	// Extract custom messages from the input struct (Query, Path, Body, etc.)
	customMessages := bind.GetCustomMessages(reflect.TypeFor[In]())

	for _, opt := range opts {
		opt(&meta)
	}

	// Pre-determine response type for optimization.
	outType := reflect.TypeFor[Out]()
	isReader := outType.Implements(reflect.TypeFor[io.Reader]())
	baseOutType := outType
	for baseOutType.Kind() == reflect.Pointer {
		baseOutType = baseOutType.Elem()
	}
	isStream := baseOutType == reflect.TypeFor[Stream]()
	isSSE := baseOutType == reflect.TypeFor[SSE]()

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
				if bindErr.Source == "auth" {
					handleError(app, w, r, problem.Unauthorized(bindErr.Err.Error()))
				} else if _, ok := errors.AsType[*http.MaxBytesError](bindErr.Err); ok {
					handleError(app, w, r, bindErr.Err)
				} else {
					handleError(app, w, r, problem.ValidationProblem("Request extraction or validation failed", []problem.InvalidParam{
						{
							Name:   bindErr.Field,
							In:     bindErr.Source,
							Reason: bindErr.Err.Error(),
						},
					}))
				}
			} else {
				if prob, ok := errors.AsType[*problem.Details](err); ok {
					handleError(app, w, r, prob)
				} else {
					handleError(app, w, r, problem.BadRequest(err.Error()))
				}
			}
			return
		}

		// 2. Run validator if present.
		if app.validator != nil {
			if err := app.validator.Struct(in); err != nil {
				if vErr, ok := errors.AsType[validator.ValidationErrors](err); ok {
					handleError(
						app,
						w,
						r,
						problem.ValidationProblem(
							"Input validation failed",
							problem.FromValidationErrors(vErr, customMessages),
						),
					)
				} else {
					handleError(app, w, r, problem.BadRequest(err.Error()))
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
			s, ok := streamResponse(out)
			if !ok {
				handleError(
					app,
					w,
					r,
					problem.Problemf(
						http.StatusInternalServerError,
						"Internal Server Error",
						"stream response is nil",
					),
				)
				return
			}
			render.Reader(w, meta.status, s.Reader, s.ContentType)
			return
		}
		if isSSE {
			sse, ok := sseResponse(out)
			if !ok {
				handleError(
					app,
					w,
					r,
					problem.Problemf(
						http.StatusInternalServerError,
						"Internal Server Error",
						"sse response is nil",
					),
				)
				return
			}
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
	finalHandler := wrapHandler(h, meta.middleware)
	groupMW := r.Middleware()
	finalHandler = wrapHandler(finalHandler, groupMW)

	fullPattern := r.Prefix() + pattern
	route := &Route{
		Method:      method,
		Pattern:     fullPattern,
		Status:      meta.status,
		Summary:     meta.summary,
		Description: meta.description,
		OperationID: meta.operationID,
		Deprecated:  meta.deprecated,
		Internal:    meta.internal,
		Tags:        meta.tags,
		Security:    meta.security,
		Schema:      schema,
		OutputType:  outType,
		middleware: append(
			append([]func(http.Handler) http.Handler{}, groupMW...),
			meta.middleware...,
		),
	}

	// Auto-register security schemes from auth extraction.
	for _, auth := range schema.Auth {
		s := SecurityScheme{
			Type:         auth.Type,
			Description:  auth.Description,
			Scheme:       auth.Scheme,
			BearerFormat: auth.BearerFmt,
			In:           auth.In,
			Name:         auth.ParamName,
		}
		app.AddSecurityScheme(auth.Name, s)
		// Auto-add security requirement if route doesn't already have one.
		if len(route.Security) == 0 {
			route.Security = append(route.Security, map[string][]string{auth.Name: {}})
		}
	}

	// Register with the router.
	r.Handle(method, pattern, finalHandler, route)

	return nil
}

func handleError(app *App, w http.ResponseWriter, r *http.Request, err error) {
	for _, observer := range app.errorObservers {
		observer(r.Context(), err)
	}

	if app.errorHandler != nil {
		app.errorHandler(w, r, err)
		return
	}

	if prob, ok := errors.AsType[*problem.Details](err); ok {
		render.Problem(w, prob.Status, prob)
	} else if maxBytesErr, ok := errors.AsType[*http.MaxBytesError](err); ok {
		render.Problem(
			w,
			http.StatusRequestEntityTooLarge,
			problem.PayloadTooLarge(
				"request body exceeds maximum allowed size of "+humanBytes(maxBytesErr.Limit),
			),
		)
	} else {
		// Default behavior for non-Problem errors
		render.Problem(w, http.StatusInternalServerError, problem.Problemf(http.StatusInternalServerError, "Internal Server Error", "%s", err.Error()))
	}
}

func humanBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return formatBytes(b, gb, "GB")
	case b >= mb:
		return formatBytes(b, mb, "MB")
	case b >= kb:
		return formatBytes(b, kb, "KB")
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}

func formatBytes(n, unit int64, suffix string) string {
	if n%unit == 0 {
		return strconv.FormatInt(n/unit, 10) + " " + suffix
	}
	quo := n / unit
	rem := (n % unit * 10) / unit
	return strconv.FormatInt(quo, 10) + "." + strconv.FormatInt(rem, 10) + " " + suffix
}

func streamResponse(out any) (Stream, bool) {
	switch s := out.(type) {
	case Stream:
		return s, true
	case *Stream:
		if s == nil {
			return Stream{}, false
		}
		return *s, true
	default:
		return Stream{}, false
	}
}

func sseResponse(out any) (SSE, bool) {
	switch s := out.(type) {
	case SSE:
		return s, true
	case *SSE:
		if s == nil {
			return SSE{}, false
		}
		return *s, true
	default:
		return SSE{}, false
	}
}
