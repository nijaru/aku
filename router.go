package aku

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"

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

	// Compile the extractor and schema once at startup.
	extractor, schema := bind.Compiler[In]()
	meta.schema = schema

	for _, opt := range opts {
		opt(&meta)
	}

	// Define the wrapper handler.
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in In
		v := reflect.ValueOf(&in).Elem()

		// 1. Extract and bind parameters.
		if err := extractor(r.Context(), r, v); err != nil {
			var bindErr *bind.BindError
			if errors.As(err, &bindErr) {
				handleError(app, w, r, ValidationProblem("Request extraction or validation failed", []InvalidParam{
					{
						Name:   bindErr.Field,
						In:     bindErr.Source,
						Reason: bindErr.Err.Error(),
					},
				}))
			} else {
				var prob *Problem
				if errors.As(err, &prob) {
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
				var vErr validator.ValidationErrors
				if errors.As(err, &vErr) {
					handleError(app, w, r, ValidationProblem("Input validation failed", FromValidationErrors(vErr)))
				} else {
					handleError(app, w, r, BadRequest(err.Error()))
				}
				return
			}
		}

		// 3. Call the user handler.
		out, err := handler(r.Context(), in)
		if err != nil {
			handleError(app, w, r, err)
			return
		}

		// 3. Render success response.
		if meta.status == http.StatusNoContent {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Handle streaming and special types
		switch v := any(out).(type) {
		case io.Reader:
			render.Reader(w, meta.status, v, "application/octet-stream")
			return
		case Stream:
			render.Reader(w, meta.status, v.Reader, v.ContentType)
			return
		case SSE:
			events := make(chan render.SSEEvent)
			go func() {
				defer close(events)
				for e := range v.Events {
					events <- render.SSEEvent{
						ID:    e.ID,
						Event: e.Event,
						Data:  e.Data,
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
		Schema:      schema,
		OutputType:  reflect.TypeOf((*Out)(nil)).Elem(),
	})

	return nil
}

func handleError(app *App, w http.ResponseWriter, r *http.Request, err error) {
	if app.errorHandler != nil {
		app.errorHandler(w, r, err)
		return
	}

	var prob *Problem
	if errors.As(err, &prob) {
		render.Problem(w, prob.Status, prob)
	} else {
		// Default behavior for non-Problem errors
		render.Problem(w, http.StatusInternalServerError, Problemf(http.StatusInternalServerError, "Internal Server Error", "%s", err.Error()))
	}
}
