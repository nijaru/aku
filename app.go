package aku

import (
	"net/http"
	"reflect"

	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/internal/openapi"
)



// Option configures an App instance.
type Option func(*App)

// App is the core framework application, wrapping a standard library HTTP multiplexer.
type App struct {
	mux        *http.ServeMux
	middleware []func(http.Handler) http.Handler
	routes     []*Route
}

// Route represents a registered route and its metadata.
type Route struct {
	Method      string
	Pattern     string
	Status      int
	Summary     string
	Description string
	Tags        []string
	Schema      *bind.Schema
	OutputType  reflect.Type
}

func (r *Route) GetMethod() string              { return r.Method }
func (r *Route) GetPattern() string             { return r.Pattern }
func (r *Route) GetStatus() int                 { return r.Status }
func (r *Route) GetSummary() string             { return r.Summary }
func (r *Route) GetDescription() string          { return r.Description }
func (r *Route) GetTags() []string              { return r.Tags }
func (r *Route) GetSchema() *bind.Schema        { return r.Schema }
func (r *Route) GetOutputType() reflect.Type    { return r.OutputType }

// New creates a new Aku application.
func New(opts ...Option) *App {
	a := &App{
		mux: http.NewServeMux(),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Use adds global middleware to the application.
func (a *App) Use(mw ...func(http.Handler) http.Handler) {
	a.middleware = append(a.middleware, mw...)
}

// Routes returns the list of registered routes and their metadata.
func (a *App) Routes() []*Route {
	return a.routes
}

// OpenAPI generates an OpenAPI 3.0 document for the application.
func (a *App) OpenAPI(title, version string) *openapi.Document {
	iroutes := make([]openapi.Route, len(a.routes))
	for i, r := range a.routes {
		iroutes[i] = r
	}
	return openapi.Generate(title, version, iroutes)
}

// OpenAPIHandler returns an http.Handler that serves the OpenAPI JSON specification.
func (a *App) OpenAPIHandler(title, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := a.OpenAPI(title, version)
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

// ServeHTTP implements the standard library http.Handler interface.

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var finalHandler http.Handler = a.mux
	for i := len(a.middleware) - 1; i >= 0; i-- {
		finalHandler = a.middleware[i](finalHandler)
	}
	finalHandler.ServeHTTP(w, r)
}
