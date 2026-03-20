package aku_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
	"github.com/nijaru/aku/middleware"
)

func TestMiddleware_Recover(t *testing.T) {
	app := aku.New()
	app.Use(middleware.Recover)

	aku.Get(app, "/panic", func(ctx context.Context, in struct{}) (string, error) {
		panic("boom")
	})

	testutil.Test(t, app).
		Get("/panic").
		ExpectStatus(http.StatusInternalServerError).
		ExpectJSON(map[string]any{
			"type":   "https://aku.sh/problems/internal-error",
			"title":  "Internal Server Error",
			"status": float64(500),
		})
}

func TestMiddleware_Timeout(t *testing.T) {
	app := aku.New()
	app.Use(middleware.Timeout(10 * time.Millisecond))

	aku.Get(app, "/slow", func(ctx context.Context, in struct{}) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return "done", nil
		}
	})

	testutil.Test(t, app).
		Get("/slow").
		ExpectStatus(http.StatusInternalServerError)
}

func TestMiddleware_CORS(t *testing.T) {
	app := aku.New()
	app.Use(middleware.CORS(middleware.CORSOptions{
		AllowedOrigins: []string{"http://example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
	}))

	aku.Get(app, "/cors", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	t.Run("Valid Origin", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/cors").
			WithHeader("Origin", "http://example.com").
			ExpectStatus(http.StatusOK).
			ExpectHeader("Access-Control-Allow-Origin", "http://example.com")
	})

	t.Run("Invalid Origin", func(t *testing.T) {
		resp := testutil.Test(t, app).
			Get("/cors").
			WithHeader("Origin", "http://attacker.com").
			Do()

		if resp.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("expected no CORS header for invalid origin")
		}
	})

	t.Run("Preflight", func(t *testing.T) {
		testutil.Test(t, app).
			Options("/cors").
			WithHeader("Origin", "http://example.com").
			WithHeader("Access-Control-Request-Method", "POST").
			ExpectStatus(http.StatusNoContent).
			ExpectHeader("Access-Control-Allow-Methods", "GET, POST")
	})
}
