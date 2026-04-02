package aku

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/problem"
)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mux                *http.ServeMux
	handler            http.Handler
	middleware         []func(http.Handler) http.Handler
	routes             []*Route
	validator          Validator
	errorHandler       ErrorHandler
	securitySchemes    map[string]SecurityScheme
	MaxMultipartMemory int64
	ShutdownTimeout    time.Duration
	bindConfig         *bind.Config
	errorObservers     []func(context.Context, error)
}

// New creates a new Aku application.
func New(opts ...Option) *App {
	a := &App{
		mux:                http.NewServeMux(),
		securitySchemes:    make(map[string]SecurityScheme),
		MaxMultipartMemory: 32 << 20, // 32MB default
		ShutdownTimeout:    30 * time.Second,
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

// Handle implements the Router interface.
func (a *App) Handle(method, pattern string, handler http.Handler, route *Route) {
	a.mux.Handle(method+" "+pattern, handler)
	a.routes = append(a.routes, route)
}

func (a *App) handleHTTP(
	method, pattern string,
	handler http.Handler,
	parentMiddleware []func(http.Handler) http.Handler,
	opts ...RouteOption,
) {
	meta := defaultRouteMeta()
	for _, opt := range opts {
		opt(&meta)
	}

	finalHandler := wrapHandler(handler, meta.middleware)
	finalHandler = wrapHandler(finalHandler, parentMiddleware)

	a.mux.Handle(method+" "+pattern, finalHandler)

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
			append([]func(http.Handler) http.Handler{}, parentMiddleware...),
			meta.middleware...,
		),
	}
	a.routes = append(a.routes, route)
}

// HandleHTTP registers a standard http.Handler on the application's multiplexer.
// This is useful for integrating third-party handlers like Prometheus or health checks
// that don't follow the typed Handler[In, Out] pattern. Route options such as
// middleware, status, tags, and security are still applied.
func (a *App) HandleHTTP(method, pattern string, handler http.Handler, opts ...RouteOption) {
	a.handleHTTP(method, pattern, handler, nil, opts...)
}

// Metrics registers a standard http.Handler for serving metrics (e.g., Prometheus).
func (a *App) Metrics(pattern string, handler http.Handler, opts ...RouteOption) {
	a.handleHTTP(http.MethodGet, pattern, handler, nil, opts...)
}

// WS satisfies the Router interface for WebSockets.
func (a *App) WS(pattern string, handler any, opts ...RouteOption) error {
	// This will be called by registerWS which we can also define if we want a generic helper.
	// But actually our WS[In, Msg] function is already generic and public.
	// To satisfy the interface we need a non-generic method that takes `any`.
	// This is slightly tricky with Go generics.
	panic("use aku.WS(router, pattern, handler) instead")
}

func (a *App) App() *App                                     { return a }
func (a *App) Prefix() string                                { return "" }
func (a *App) Middleware() []func(http.Handler) http.Handler { return nil }

// Routes returns the list of registered routes and their metadata.
func (a *App) Routes() []*Route {
	return a.routes
}

// AddSecurityScheme adds a security scheme to the application.
func (a *App) AddSecurityScheme(name string, scheme SecurityScheme) {
	a.securitySchemes[name] = scheme
}

// AddErrorObserver adds an error observer to the application.
// Error observers are called whenever an error is handled by the framework.
func (a *App) AddErrorObserver(observer func(context.Context, error)) {
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
	iw.written = false
	iw.intercepted = false
	iw.hijacked = false
	defer errorInterceptorPool.Put(iw)

	finalHandler := a.handler
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
	written     bool
	intercepted bool
	hijacked    bool
}

func (i *errorInterceptor) WriteHeader(status int) {
	if i.written || i.hijacked {
		return
	}
	i.status = status
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
		if (i.status == http.StatusNotFound && string(b) == "404 page not found\n") ||
			(i.status == http.StatusMethodNotAllowed && string(b) == "Method Not Allowed\n") {
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
	if f, ok := i.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Run starts the HTTP server on the given address with graceful shutdown support.
// It listens for SIGINT and SIGTERM signals and waits for active requests to finish
// up to the configured ShutdownTimeout.
func (a *App) Run(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: a,
	}

	idleConnsClosed := make(chan struct{})
	serverError := make(chan error, 1)

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

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
