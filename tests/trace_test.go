package aku_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
	"github.com/nijaru/aku/middleware"
)

func TestMiddleware_RequestID(t *testing.T) {
	app := aku.New()
	app.Use(middleware.RequestID)

	aku.Get(app, "/trace", func(ctx context.Context, _ struct{}) (string, error) {
		return middleware.GetRequestID(ctx), nil
	})

	t.Run("Generates new ID", func(t *testing.T) {
		resp := testutil.Test(t, app).
			Get("/trace").
			Do()

		resp.ExpectStatus(http.StatusOK)
		id := resp.Header().Get("X-Request-ID")
		if id == "" {
			t.Fatal("expected X-Request-ID header to be set")
		}

		if string(bytes.TrimSpace(resp.Body())) != "\""+id+"\"" { // JSON quoted
			t.Errorf("expected body to contain the same ID, got %s", resp.Body())
		}
	})

	t.Run("Propagates existing ID", func(t *testing.T) {
		existingID := "test-id-123"
		resp := testutil.Test(t, app).
			Get("/trace").
			WithHeader("X-Request-ID", existingID).
			Do()

		resp.ExpectStatus(http.StatusOK)
		id := resp.Header().Get("X-Request-ID")
		if id != existingID {
			t.Errorf("expected X-Request-ID to be %s, got %s", existingID, id)
		}
	})
}
