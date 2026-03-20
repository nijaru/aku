package aku

import (
	"net/http"
	"time"

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
// If enabled, requests with unknown query parameters will return a 400 Bad Request.
func WithStrictQuery() Option {
	return func(a *App) {
		if a.bindConfig == nil {
			a.bindConfig = &bind.Config{}
		}
		a.bindConfig.StrictQuery = true
	}
}

// WithStrictHeader enables strict mode for header parameters.
// If enabled, requests with unknown header parameters will return a 400 Bad Request.
func WithStrictHeader() Option {
	return func(a *App) {
		if a.bindConfig == nil {
			a.bindConfig = &bind.Config{}
		}
		a.bindConfig.StrictHeader = true
	}
}
