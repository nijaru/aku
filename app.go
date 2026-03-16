package aku

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/internal/openapi"
)

// SecurityScheme describes an authentication scheme for the API.
type SecurityScheme struct {
	Type             string
	Description      string
	Name             string // for apiKey
	In               string // for apiKey: "query", "header", "cookie"
	Scheme           string // for http
	BearerFormat     string // for http ("bearer")
	OpenIdConnectUrl string // for openIdConnect
}

// Validator is the interface that wraps the basic Validate method.
type Validator interface {
	Struct(s any) error
}

// ErrorHandler is a function that handles errors returned by handlers or the framework.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// Option configures an App instance.
type Option func(*App)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mux                *http.ServeMux
	middleware         []func(http.Handler) http.Handler
	routes             []*Route
	validator          Validator
	errorHandler       ErrorHandler
	securitySchemes    map[string]SecurityScheme
	MaxMultipartMemory int64
	ShutdownTimeout    time.Duration
	bindConfig         *bind.Config
}

// Route represents a registered route and its metadata.
type Route struct {
	Method      string
	Pattern     string
	Status      int
	Summary     string
	Description string
	Tags        []string
	Security    []map[string][]string
	Schema      *bind.Schema
	OutputType  reflect.Type
	middleware  []func(http.Handler) http.Handler
}

func (r *Route) GetMethod() string              { return r.Method }
func (r *Route) GetPattern() string             { return r.Pattern }
func (r *Route) GetStatus() int                 { return r.Status }
func (r *Route) GetSummary() string             { return r.Summary }
func (r *Route) GetDescription() string          { return r.Description }
func (r *Route) GetTags() []string              { return r.Tags }
func (r *Route) GetSecurity() []map[string][]string { return r.Security }
func (r *Route) GetSchema() *bind.Schema        { return r.Schema }
func (r *Route) GetOutputType() reflect.Type    { return r.OutputType }

// New creates a new Aku application.
func New(opts ...Option) *App {
	a := &App{
		mux:                http.NewServeMux(),
		securitySchemes:    make(map[string]SecurityScheme),
		MaxMultipartMemory: 32 << 20, // 32MB default
		ShutdownTimeout:    30 * time.Second,
	}

	for _, opt := range opts {
		opt(a)
	}

	a.bindConfig = &bind.Config{
		MaxMultipartMemory: a.MaxMultipartMemory,
	}

	return a
}

// Use adds global middleware to the application.
func (a *App) Use(mw ...func(http.Handler) http.Handler) {
	a.middleware = append(a.middleware, mw...)
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

func (a *App) App() *App                            { return a }
func (a *App) Prefix() string                       { return "" }
func (a *App) Middleware() []func(http.Handler) http.Handler { return nil }

func (a *App) Static(prefix, root string) {
	a.StaticFS(prefix, http.Dir(root))
}

func (a *App) StaticFS(prefix string, fs http.FileSystem) {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	handler := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(fs))
	a.mux.Handle(prefix, handler)
}

// WithGlobalMiddleware adds global middleware to the application.
func WithGlobalMiddleware(mw ...func(http.Handler) http.Handler) Option {
	return func(a *App) {
		a.middleware = append(a.middleware, mw...)
	}
}

// WithValidator sets a custom validator for the application.
func WithValidator(v Validator) Option {
	return func(a *App) {
		a.validator = v
	}
}

// WithErrorHandler sets a custom error handler for the application.
func WithErrorHandler(h ErrorHandler) Option {
	return func(a *App) {
		a.errorHandler = h
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown.
func WithShutdownTimeout(d time.Duration) Option {
	return func(a *App) {
		a.ShutdownTimeout = d
	}
}

// WithMaxMultipartMemory sets the maximum memory to use for multipart forms.
func WithMaxMultipartMemory(max int64) Option {
	return func(a *App) {
		a.MaxMultipartMemory = max
	}
}

// Routes returns the list of registered routes and their metadata.
func (a *App) Routes() []*Route {
	return a.routes
}

// AddSecurityScheme adds a security scheme to the application.
func (a *App) AddSecurityScheme(name string, scheme SecurityScheme) {
	a.securitySchemes[name] = scheme
}

// OpenAPIDocument generates an OpenAPI 3.0 document for the application.
func (a *App) OpenAPIDocument(title, version string) *openapi.Document {
	iroutes := make([]openapi.Route, len(a.routes))
	for i, r := range a.routes {
		iroutes[i] = r
	}

	schemes := make(map[string]openapi.SecurityScheme)
	for name, s := range a.securitySchemes {
		schemes[name] = openapi.SecurityScheme{
			Type:             s.Type,
			Description:      s.Description,
			Name:             s.Name,
			In:               s.In,
			Scheme:           s.Scheme,
			BearerFormat:     s.BearerFormat,
			OpenIdConnectUrl: s.OpenIdConnectUrl,
		}
	}

	return openapi.Generate(title, version, iroutes, schemes)
}

// OpenAPI registers an endpoint that serves the OpenAPI JSON specification.
func (a *App) OpenAPI(pattern, title, version string) {
	a.mux.Handle("GET "+pattern, a.OpenAPIHandler(title, version))
}

// SwaggerUI registers an endpoint that serves the Swagger UI.
func (a *App) SwaggerUI(pattern, specURL string) {
	a.mux.Handle("GET "+pattern, a.SwaggerUIHandler(specURL))
}

// RedocUI registers an endpoint that serves the Redoc UI.
func (a *App) RedocUI(pattern, specURL string) {
	a.mux.Handle("GET "+pattern, a.RedocUIHandler(specURL))
}

// OpenAPIHandler returns an http.Handler that serves the OpenAPI JSON specification.
func (a *App) OpenAPIHandler(title, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := a.OpenAPIDocument(title, version)
		data, err := doc.JSON()
		if err != nil {
			http.Error(w, "Failed to generate OpenAPI spec", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}

// SwaggerUIHandler returns an http.Handler that serves the Swagger UI.
// The specURL is the URL where the OpenAPI JSON is served (e.g., "/openapi.json").
func (a *App) SwaggerUIHandler(specURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui.css" >
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-bundle.js"> </script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-standalone-preset.js"> </script>
    <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: "` + specURL + `",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout"
      });
      window.ui = ui;
    };
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})
}

// RedocUIHandler returns an http.Handler that serves the Redoc UI.
// The specURL is the URL where the OpenAPI JSON is served (e.g., "/openapi.json").
func (a *App) RedocUIHandler(specURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Redoc</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
    <redoc spec-url="` + specURL + `"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"> </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})
}

// ServeHTTP implements the standard library http.Handler interface.

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	iw := &errorInterceptor{ResponseWriter: w}
	var finalHandler http.Handler = a.mux

	for i := len(a.middleware) - 1; i >= 0; i-- {
		finalHandler = a.middleware[i](finalHandler)
	}

	finalHandler.ServeHTTP(iw, r)

	if !iw.written && (iw.status == http.StatusNotFound || iw.status == http.StatusMethodNotAllowed) {
		// If mux wrote a standard error, replace it with a Problem
		var prob *Problem
		if iw.status == http.StatusNotFound {
			prob = NotFound("The requested resource was not found")
		} else {
			prob = Problemf(http.StatusMethodNotAllowed, "Method Not Allowed", "The %s method is not allowed for this resource", r.Method)
		}
		handleError(a, w, r, prob)
	}
}

type errorInterceptor struct {
	http.ResponseWriter
	status  int
	written bool
}

func (i *errorInterceptor) WriteHeader(status int) {
	i.status = status
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		i.written = true
		i.ResponseWriter.WriteHeader(status)
	}
}

func (i *errorInterceptor) Write(b []byte) (int, error) {
	if i.status == http.StatusNotFound || i.status == http.StatusMethodNotAllowed {
		// Suppress the standard library's plain text response
		return len(b), nil
	}
	i.written = true
	return i.ResponseWriter.Write(b)
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
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		slog.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("HTTP server Shutdown", slog.Any("error", err))
		}
		close(idleConnsClosed)
	}()

	slog.Info("Serving on " + addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	<-idleConnsClosed
	slog.Info("Server stopped")
	return nil
}
