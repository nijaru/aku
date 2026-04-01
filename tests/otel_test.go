package aku_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
	"github.com/nijaru/aku/middleware/otelaku"
	"github.com/nijaru/aku/problem"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTelMiddleware(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(sr))

	app := aku.New()
	app.AddErrorObserver(otelaku.ErrorObserver())
	app.Use(otelaku.Middleware(otelaku.WithTracerProvider(tp)))

	aku.Get(app, "/users/{id}", func(ctx context.Context, in struct {
		Path struct {
			ID string `path:"id"`
		}
	},
	) (string, error) {
		return "hello " + in.Path.ID, nil
	})

	aku.Get(app, "/error", func(ctx context.Context, _ struct{}) (string, error) {
		return "", problem.BadRequest("bad request")
	})

	t.Run("Names span with route pattern", func(t *testing.T) {
		sr.Reset()
		testutil.Test(t, app).
			Get("/users/123").
			ExpectStatus(http.StatusOK).
			ExpectJSON("hello 123")

		spans := sr.Ended()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}
		if spans[0].Name() != "GET /users/{id}" {
			t.Errorf("expected span name 'GET /users/{id}', got '%s'", spans[0].Name())
		}
	})

	t.Run("Records error on span", func(t *testing.T) {
		sr.Reset()
		testutil.Test(t, app).
			Get("/error").
			ExpectStatus(http.StatusBadRequest)

		spans := sr.Ended()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}
		events := spans[0].Events()
		found := false
		for _, e := range events {
			if e.Name == "exception" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected error to be recorded on span")
		}

		// Check attributes
		attrs := spans[0].Attributes()
		attrMap := make(map[string]string)
		for _, a := range attrs {
			attrMap[string(a.Key)] = a.Value.AsString()
		}

		if attrMap["http.problem.title"] != "Bad Request" {
			t.Errorf(
				"expected http.problem.title 'Bad Request', got '%s'",
				attrMap["http.problem.title"],
			)
		}
	})
}
