package aku_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/middleware/otelaku"
	"go.opentelemetry.io/otel/trace/noop"
)

func BenchmarkOTelMiddleware(b *testing.B) {
	tp := noop.NewTracerProvider()
	app := aku.New()
	app.Use(otelaku.Middleware(otelaku.WithTracerProvider(tp)))

	aku.Get(app, "/bench", func(ctx context.Context, _ struct{}) (string, error) {
		return "ok", nil
	})

	req := httptest.NewRequest("GET", "/bench", nil)
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

func BenchmarkOTelPerRoute(b *testing.B) {
	tp := noop.NewTracerProvider()
	app := aku.New()
	mw := otelaku.Middleware(otelaku.WithTracerProvider(tp))

	aku.Get(app, "/bench", func(ctx context.Context, _ struct{}) (string, error) {
		return "ok", nil
	}, aku.WithMiddleware(mw))

	req := httptest.NewRequest("GET", "/bench", nil)
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

func BenchmarkNoOTel(b *testing.B) {
	app := aku.New()

	aku.Get(app, "/bench", func(ctx context.Context, _ struct{}) (string, error) {
		return "ok", nil
	})

	req := httptest.NewRequest("GET", "/bench", nil)
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.reset()
		app.ServeHTTP(w, req)
	}
}
