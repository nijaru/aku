package aku_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
	"github.com/nijaru/aku/middleware"
)

func TestMiddleware_Limit(t *testing.T) {
	app := aku.New()
	// Allow 1 request per second, with burst of 1
	app.Use(middleware.Limit(1, 1))

	aku.Get(app, "/limited", func(ctx context.Context, _ struct{}) (string, error) {
		return "ok", nil
	})

	t.Run("Allow first request", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/limited").
			ExpectStatus(http.StatusOK)
	})

	t.Run("Deny second request immediately", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/limited").
			ExpectStatus(http.StatusTooManyRequests)
	})
}
