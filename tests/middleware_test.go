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

func TestMiddleware_SecurityHeaders_Defaults(t *testing.T) {
	app := aku.New()
	app.Use(middleware.SecurityHeaders())

	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	testutil.Test(t, app).
		Get("/secure").
		ExpectStatus(http.StatusOK).
		ExpectHeader("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'").
		ExpectHeader("Strict-Transport-Security", "max-age=63072000").
		ExpectHeader("X-Frame-Options", "DENY").
		ExpectHeader("X-Content-Type-Options", "nosniff").
		ExpectHeader("Referrer-Policy", "strict-origin-when-cross-origin").
		ExpectHeader("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=(), interest-cohort=()").
		ExpectHeader("Cross-Origin-Opener-Policy", "same-origin").
		ExpectHeader("Cross-Origin-Resource-Policy", "same-origin").
		ExpectHeader("X-XSS-Protection", "0")
}

func TestMiddleware_SecurityHeaders_Custom(t *testing.T) {
	app := aku.New()
	app.Use(middleware.SecurityHeaders(middleware.SecurityHeadersOptions{
		ContentSecurityPolicy:     "default-src 'none'",
		HSTSMaxAge:                63072000,
		HSTSIncludeSubDomains:     true,
		HSTSPreload:               true,
		XFrameOptions:             "SAMEORIGIN",
		ReferrerPolicy:            "no-referrer",
		CrossOriginEmbedderPolicy: "require-corp",
	}))

	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	testutil.Test(t, app).
		Get("/secure").
		ExpectStatus(http.StatusOK).
		ExpectHeader("Content-Security-Policy", "default-src 'none'").
		ExpectHeader("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload").
		ExpectHeader("X-Frame-Options", "SAMEORIGIN").
		ExpectHeader("X-Content-Type-Options", "nosniff").
		ExpectHeader("Referrer-Policy", "no-referrer").
		ExpectHeader("Cross-Origin-Embedder-Policy", "require-corp")
}

func TestMiddleware_SecurityHeaders_DisabledHSTS(t *testing.T) {
	app := aku.New()
	app.Use(middleware.SecurityHeaders(middleware.SecurityHeadersOptions{
		HSTSMaxAge: -1,
	}))

	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	resp := testutil.Test(t, app).
		Get("/secure").
		Do()

	if resp.Header().Get("Strict-Transport-Security") != "" {
		t.Error("expected no HSTS header when disabled")
	}
}

func TestMiddleware_SecurityHeaders_NoCOEPByDefault(t *testing.T) {
	app := aku.New()
	app.Use(middleware.SecurityHeaders())

	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	resp := testutil.Test(t, app).
		Get("/secure").
		Do()

	if resp.Header().Get("Cross-Origin-Embedder-Policy") != "" {
		t.Error("expected no COEP header by default (too aggressive for general use)")
	}
}

func TestMiddleware_SecurityHeaders_EmptyOptions(t *testing.T) {
	app := aku.New()
	app.Use(middleware.SecurityHeaders(middleware.SecurityHeadersOptions{}))

	aku.Get(app, "/secure", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	testutil.Test(t, app).
		Get("/secure").
		ExpectStatus(http.StatusOK).
		ExpectHeader("X-Content-Type-Options", "nosniff")
}

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
