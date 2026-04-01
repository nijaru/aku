package otelaku

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	tracer         trace.Tracer
}

// Option allows configuring the OpenTelemetry middleware.
type Option func(*config)

// WithTracerProvider sets the TracerProvider for the middleware.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		c.tracerProvider = tp
	}
}

// WithPropagator sets the Propagator for the middleware.
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		c.propagator = p
	}
}

var methodSpans = map[string]string{
	"GET":     "HTTP GET",
	"POST":    "HTTP POST",
	"PUT":     "HTTP PUT",
	"PATCH":   "HTTP PATCH",
	"DELETE":  "HTTP DELETE",
	"HEAD":    "HTTP HEAD",
	"OPTIONS": "HTTP OPTIONS",
}

// Middleware creates a high-performance OpenTelemetry tracing middleware.
// It is optimized for Go 1.22+ and uses the matched route pattern as the span name.
func Middleware(opts ...Option) func(http.Handler) http.Handler {
	c := &config{
		tracerProvider: otel.GetTracerProvider(),
		propagator:     otel.GetTextMapPropagator(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.tracer = c.tracerProvider.Tracer("github.com/nijaru/aku")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract remote trace context.
			ctx := c.propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start the span.
			// If this is a route-level middleware (inside mux), r.Pattern is already set.
			// If this is a global middleware (outside mux), r.Pattern is empty and we'll update it later.
			spanName := r.Pattern
			if spanName == "" {
				var ok bool
				spanName, ok = methodSpans[r.Method]
				if !ok {
					spanName = "HTTP " + r.Method
				}
			}

			ctx, span := c.tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			// Pass the span context down.
			r = r.WithContext(ctx)

			// Call the next handler.
			next.ServeHTTP(w, r)

			// If global, we might now have a pattern from the mux.
			if r.Pattern != "" && spanName != r.Pattern {
				span.SetName(r.Pattern)
			}
		})
	}
}

// ErrorObserver returns an error observer that records errors to the OpenTelemetry span in the context.
// If the error is an RFC 9457 Problem Detail, it records additional attributes.
func ErrorObserver() func(context.Context, error) {
	return func(ctx context.Context, err error) {
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			span.RecordError(err)

			// Record rich attributes for Problem Details
			if prob, ok := err.(interface {
				GetStatus() int
				GetTitle() string
				GetType() string
				GetDetail() string
			}); ok {
				span.SetAttributes(
					attribute.Int("http.problem.status", prob.GetStatus()),
					attribute.String("http.problem.title", prob.GetTitle()),
					attribute.String("http.problem.type", prob.GetType()),
				)
				if detail := prob.GetDetail(); detail != "" {
					span.SetAttributes(attribute.String("http.problem.detail", detail))
				}
			}
		}
	}
}
