package aku

import (
	"net/http"
	"reflect"

	"github.com/nijaru/aku/internal/bind"
	"github.com/nijaru/aku/internal/openapi"
)

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
	mux          *http.ServeMux
	middleware   []func(http.Handler) http.Handler
	routes       []*Route
	validator    Validator
	errorHandler ErrorHandler
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
	var finalHandler http.Handler = a.mux
	for i := len(a.middleware) - 1; i >= 0; i-- {
		finalHandler = a.middleware[i](finalHandler)
	}
	finalHandler.ServeHTTP(w, r)
}
