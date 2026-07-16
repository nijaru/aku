// Package aku is a high-performance, typesafe HTTP framework for building APIs
// on Go's standard library net/http.
//
// Aku bridges the gap between net/http and the ergonomics of modern frameworks
// like FastAPI or Axum. It uses Go's type system to automate request extraction,
// validation, and OpenAPI documentation without sacrificing standard library
// compatibility.
//
// # Quick Start
//
//	type GreetRequest struct {
//	    Path struct {
//	        Name string `path:"name"`
//	    }
//	    Query struct {
//	        Shout bool `query:"shout"`
//	    }
//	}
//
//	type GreetResponse struct {
//	    Message string `json:"message"`
//	}
//
//	func Greet(ctx context.Context, in GreetRequest) (GreetResponse, error) {
//	    msg := "Hello, " + in.Path.Name
//	    if in.Query.Shout {
//	        msg += "!"
//	    }
//	    return GreetResponse{Message: msg}, nil
//	}
//
//	func main() {
//	    app := aku.New()
//	    if err := aku.Get(app, "/greet/{name}", Greet); err != nil {
//	        log.Fatal(err)
//	    }
//	    if err := app.OpenAPI("/openapi.json", "My API", "1.0.0"); err != nil {
//	        log.Fatal(err)
//	    }
//	    if err := app.SwaggerUI("/docs", "/openapi.json"); err != nil {
//	        log.Fatal(err)
//	    }
//	    log.Fatal(app.Run(":8080"))
//	}
//
// # Features
//
//   - Typesafe request extraction from Path, Query, Header, Form, Body, and Context
//   - Automatic OpenAPI 3.0 generation from Go types
//   - Precompiled extraction plans and coercers for lower request-time overhead
//   - Optional validation via go-playground/validator and Validate hooks
//   - Middleware suite: logging, recovery, timeout, CORS, compression, rate limiting, security headers
//   - Streaming support: io.Reader, Server-Sent Events, WebSockets
//   - Standard http.Handler escape hatches for Prometheus, health checks, etc.
package aku

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/problem"
)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mu                 sync.RWMutex
	mux                *http.ServeMux
	registrations      map[string]http.Handler
	handler            http.Handler
	middleware         []func(http.Handler) http.Handler
	routes             []*Route
	validator          Validator
	errorHandler       ErrorHandler
	securitySchemes    map[string]SecurityScheme
	openapiVersion     uint64
	MaxMultipartMemory int64
	ShutdownTimeout    time.Duration
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	bindConfig         *bind.Config
	errorObservers     []func(context.Context, error)
}

// New creates a new Aku application.
func New(opts ...Option) *App {
	a := &App{
		mux:                http.NewServeMux(),
		registrations:      make(map[string]http.Handler),
		securitySchemes:    make(map[string]SecurityScheme),
		MaxMultipartMemory: 32 << 20, // 32MB default
		ShutdownTimeout:    30 * time.Second,
		ReadHeaderTimeout:  5 * time.Second,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		IdleTimeout:        120 * time.Second,
		bindConfig:         &bind.Config{},
	}

	for _, opt := range opts {
		opt(a)
	}

	a.bindConfig.MaxMultipartMemory = a.MaxMultipartMemory
	a.refreshHandler()

	return a
}

// Use adds global middleware to the application.
func (a *App) Use(mw ...func(http.Handler) http.Handler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.middleware = append(a.middleware, mw...)
	a.refreshHandler()
}

func (a *App) refreshHandler() {
	a.handler = wrapHandler(a.mux, a.middleware)
}

// Group creates a new route group with the given prefix and middleware.
func (a *App) Group(prefix string, mw ...func(http.Handler) http.Handler) *Group {
	return &Group{
		app:        a,
		prefix:     prefix,
		middleware: mw,
	}
}

// Handle implements the Router interface. It returns an error when the
// pattern conflicts with an existing route or is otherwise invalid.
func (a *App) Handle(method, pattern string, handler http.Handler, route *Route) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.registerHandlersLocked(handlerRegistration{
		pattern: method + " " + pattern,
		handler: handler,
	}); err != nil {
		return err
	}
	if route != nil {
		a.routes = append(a.routes, route)
		a.openapiVersion++
	}
	return nil
}

func (a *App) registerHandler(pattern string, handler http.Handler) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.registerHandlersLocked(handlerRegistration{pattern: pattern, handler: handler})
}

type handlerRegistration struct {
	pattern string
	handler http.Handler
}

// registerHandlersLocked validates all new patterns against the current mux
// before mutating it. Static routes need two patterns (the exact-prefix
// redirect and the trailing-slash subtree), so treating registration as a
// batch prevents a failed second pattern from leaving a partial route behind.
func (a *App) registerHandlersLocked(registrations ...handlerRegistration) error {
	if len(registrations) == 0 {
		return nil
	}
	if len(registrations) == 1 {
		registration := registrations[0]
		if err := registerMuxHandler(a.mux, registration.pattern, registration.handler); err != nil {
			return err
		}
		a.registrations[registration.pattern] = registration.handler
		return nil
	}

	preflight := http.NewServeMux()
	for pattern, handler := range a.registrations {
		if err := registerMuxHandler(preflight, pattern, handler); err != nil {
			return err
		}
	}
	for _, registration := range registrations {
		if err := registerMuxHandler(preflight, registration.pattern, registration.handler); err != nil {
			return err
		}
	}

	for _, registration := range registrations {
		a.mux.Handle(registration.pattern, registration.handler)
		a.registrations[registration.pattern] = registration.handler
	}
	return nil
}

func registerMuxHandler(mux *http.ServeMux, pattern string, handler http.Handler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register pattern %q: %v", pattern, recovered)
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}

func (a *App) handleHTTP(
	method, pattern string,
	handler http.Handler,
	parentMiddleware []func(http.Handler) http.Handler,
	opts ...RouteOption,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register %s route %q: %v", method, pattern, recovered)
		}
	}()
	meta := defaultRouteMeta()
	for _, opt := range opts {
		opt(&meta)
	}

	finalHandler := wrapHandler(handler, meta.middleware)
	finalHandler = wrapHandler(finalHandler, parentMiddleware)

	route := &Route{
		Method:      method,
		Pattern:     pattern,
		Status:      meta.status,
		Summary:     meta.summary,
		Description: meta.description,
		Tags:        meta.tags,
		Internal:    meta.internal,
		Deprecated:  meta.deprecated,
		OperationID: meta.operationID,
		Security:    meta.security,
		middleware: append(
			slices.Clone(parentMiddleware),
			meta.middleware...,
		),
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.registerHandlersLocked(handlerRegistration{
		pattern: method + " " + pattern,
		handler: finalHandler,
	}); err != nil {
		return err
	}
	a.routes = append(a.routes, route)
	a.openapiVersion++
	return nil
}

// HandleHTTP registers a standard http.Handler on the application's multiplexer.
// This is useful for integrating third-party handlers like Prometheus or health checks
// that don't follow the typed Handler[In, Out] pattern. Route options such as
// middleware, status, tags, and security are still applied. Registration errors
// are returned to the caller.
func (a *App) HandleHTTP(method, pattern string, handler http.Handler, opts ...RouteOption) error {
	return a.handleHTTP(method, pattern, handler, nil, opts...)
}

// Metrics registers a standard http.Handler for serving metrics (e.g., Prometheus).
// Registration errors are returned to the caller.
func (a *App) Metrics(pattern string, handler http.Handler, opts ...RouteOption) error {
	return a.handleHTTP(http.MethodGet, pattern, handler, nil, opts...)
}

func (a *App) App() *App                                     { return a }
func (a *App) Prefix() string                                { return "" }
func (a *App) Middleware() []func(http.Handler) http.Handler { return nil }

// Routes returns the list of registered routes and their metadata.
func (a *App) Routes() []*Route {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.routes == nil {
		return nil
	}
	routes := make([]*Route, len(a.routes))
	for i, route := range a.routes {
		routes[i] = cloneRoute(route)
	}
	return routes
}

// AddSecurityScheme adds a security scheme to the application.
func (a *App) AddSecurityScheme(name string, scheme SecurityScheme) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.securitySchemes[name] = scheme
	a.openapiVersion++
}

// AddErrorObserver adds an error observer to the application.
// Error observers are called whenever an error is handled by the framework.
func (a *App) AddErrorObserver(observer func(context.Context, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.errorObservers = append(a.errorObservers, observer)
}

// ServeHTTP implements the standard library http.Handler interface.

var errorInterceptorPool = sync.Pool{
	New: func() any {
		return &errorInterceptor{}
	},
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	iw := errorInterceptorPool.Get().(*errorInterceptor)
	iw.ResponseWriter = w
	iw.status = 0
	iw.statusSet = false
	iw.written = false
	iw.intercepted = false
	iw.hijacked = false
	defer func() {
		iw.ResponseWriter = nil
		errorInterceptorPool.Put(iw)
	}()

	a.mu.RLock()
	finalHandler := a.handler
	a.mu.RUnlock()
	if finalHandler == nil {
		finalHandler = a.mux
	}

	finalHandler.ServeHTTP(iw, r)

	if iw.intercepted {
		// If mux wrote a standard error, replace it with a Problem
		var prob *problem.Details
		if iw.status == http.StatusNotFound {
			prob = problem.NotFound("The requested resource was not found")
		} else {
			prob = problem.Problemf(http.StatusMethodNotAllowed, "Method Not Allowed", "The %s method is not allowed for this resource", r.Method)
		}
		handleError(a, w, r, prob)
	} else if !iw.written && (iw.status == http.StatusNotFound || iw.status == http.StatusMethodNotAllowed) {
		// A handler called WriteHeader(404) but no Write(), we must write the header.
		if !iw.hijacked {
			w.WriteHeader(iw.status)
		}
	}
}

type errorInterceptor struct {
	http.ResponseWriter
	status      int
	statusSet   bool
	written     bool
	intercepted bool
	hijacked    bool
}

func (i *errorInterceptor) WriteHeader(status int) {
	if i.statusSet || i.written || i.hijacked {
		return
	}
	i.status = status
	i.statusSet = true
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		i.written = true
		i.ResponseWriter.WriteHeader(status)
	}
}

func (i *errorInterceptor) Write(b []byte) (int, error) {
	if i.hijacked {
		return 0, http.ErrHijacked
	}
	if !i.written {
		i.written = true
		if i.isServeMuxError(b) {
			i.intercepted = true
			return len(b), nil
		}
		// Write the delayed header
		if i.status == http.StatusNotFound || i.status == http.StatusMethodNotAllowed {
			i.ResponseWriter.WriteHeader(i.status)
		}
	}

	if i.intercepted {
		return len(b), nil
	}

	return i.ResponseWriter.Write(b)
}

func (i *errorInterceptor) isServeMuxError(b []byte) bool {
	if i.Header().Get("Content-Type") != "text/plain; charset=utf-8" ||
		i.Header().Get("X-Content-Type-Options") != "nosniff" {
		return false
	}
	return (i.status == http.StatusNotFound && string(b) == "404 page not found\n") ||
		(i.status == http.StatusMethodNotAllowed && string(b) == "Method Not Allowed\n")
}

func (i *errorInterceptor) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := i.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	i.hijacked = true
	return h.Hijack()
}

func (i *errorInterceptor) Unwrap() http.ResponseWriter {
	return i.ResponseWriter
}

func (i *errorInterceptor) Flush() {
	if !i.statusSet {
		i.status = http.StatusOK
		i.statusSet = true
	}
	if !i.written {
		if i.status == http.StatusNotFound || i.status == http.StatusMethodNotAllowed {
			i.ResponseWriter.WriteHeader(i.status)
		}
		i.written = true
	}
	if f, ok := i.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Run starts the HTTP server on the given address with graceful shutdown support.
// It listens for SIGINT and SIGTERM signals and waits for active requests to finish
// up to the configured ShutdownTimeout.
func (a *App) Run(addr string) error {
	srv := a.server(addr)

	idleConnsClosed := make(chan struct{})
	serverError := make(chan error, 1)

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigint)

		select {
		case <-sigint:
			slog.Info("Shutting down server...")
		case <-serverError:
			// Server failed to start or stopped unexpectedly, skip shutdown gracefully
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("HTTP server Shutdown", slog.Any("error", err))
		}
		close(idleConnsClosed)
	}()

	slog.Info("Serving on " + addr)
	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		serverError <- err
		return err
	}

	// Only wait for shutdown if the server closed intentionally
	if err == http.ErrServerClosed {
		<-idleConnsClosed
		slog.Info("Server stopped")
	}

	return nil
}

func (a *App) server(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           a,
		ReadHeaderTimeout: a.ReadHeaderTimeout,
		ReadTimeout:       a.ReadTimeout,
		WriteTimeout:      a.WriteTimeout,
		IdleTimeout:       a.IdleTimeout,
	}
}
