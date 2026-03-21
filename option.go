package aku

import (
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku/internal/bind"
)

// Option configures an App instance.
type Option func(*App)

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

		// If it's the standard go-playground validator, we can make it aware of our tags
		if gpv, ok := v.(*validator.Validate); ok {
			gpv.RegisterTagNameFunc(func(fld reflect.StructField) string {
				// Try JSON first, then Query, then Header, then Path, then Form
				for _, tag := range []string{"json", "query", "header", "path", "form"} {
					name := strings.Split(fld.Tag.Get(tag), ",")[0]
					if name != "" && name != "-" {
						return name
					}
				}
				return fld.Name
			})
		}
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

// WithStrictQuery enables strict mode for query parameters.
// If enabled, requests with unknown query parameters will return a 422 Unprocessable Entity.
func WithStrictQuery() Option {
	return func(a *App) {
		if a.bindConfig == nil {
			a.bindConfig = &bind.Config{}
		}
		a.bindConfig.StrictQuery = true
	}
}

// WithStrictHeader enables strict mode for header parameters.
// If enabled, requests with unknown header parameters will return a 422 Unprocessable Entity.
func WithStrictHeader() Option {
	return func(a *App) {
		if a.bindConfig == nil {
			a.bindConfig = &bind.Config{}
		}
		a.bindConfig.StrictHeader = true
	}
}

// RouteOption configures a specific route at registration time.
type RouteOption func(*routeMeta)

type routeMeta struct {
	status      int
	summary     string
	description string
	operationID string
	deprecated  bool
	internal    bool
	tags        []string
	security    []map[string][]string
	middleware  []func(http.Handler) http.Handler
	schema      *bind.Schema
	spa         bool
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

// WithSummary sets the summary for a route.
func WithSummary(s string) RouteOption {
	return func(m *routeMeta) {
		m.summary = s
	}
}

// WithDescription sets the description for a route.
func WithDescription(s string) RouteOption {
	return func(m *routeMeta) {
		m.description = s
	}
}

// WithOperationID sets the OpenAPI operation ID for a route.
func WithOperationID(id string) RouteOption {
	return func(m *routeMeta) {
		m.operationID = id
	}
}

// WithDeprecated marks the route as deprecated in OpenAPI.
func WithDeprecated() RouteOption {
	return func(m *routeMeta) {
		m.deprecated = true
	}
}

// WithInternal marks the route as internal, hiding it from OpenAPI.
func WithInternal() RouteOption {
	return func(m *routeMeta) {
		m.internal = true
	}
}

// WithTags adds OpenAPI tags to a route.
func WithTags(tags ...string) RouteOption {
	return func(m *routeMeta) {
		m.tags = append(m.tags, tags...)
	}
}

// WithTag adds a single OpenAPI tag to a route.
func WithTag(tag string) RouteOption {
	return func(m *routeMeta) {
		m.tags = append(m.tags, tag)
	}
}

// WithSecurity adds OpenAPI security requirements to a route.
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

// WithMiddleware adds route-local middleware.
func WithMiddleware(mw ...func(http.Handler) http.Handler) RouteOption {
	return func(m *routeMeta) {
		m.middleware = append(m.middleware, mw...)
	}
}

// WithSPA enables single-page application mode for static file serving.
// If enabled, 404s will fallback to index.html at the root of the static directory.
func WithSPA() RouteOption {
	return func(m *routeMeta) {
		m.spa = true
	}
}
