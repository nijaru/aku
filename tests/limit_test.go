package aku_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type limitedBodyInput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func TestBodySizeLimit_UnknownLengthReturns413(t *testing.T) {
	app := aku.New(aku.WithGlobalMiddleware(middleware.BodySizeLimit(middleware.BodySizeLimitConfig{
		MaxBodyBytes: 8,
	})))

	aku.Post(
		app,
		"/limited-body",
		func(ctx context.Context, in limitedBodyInput) (map[string]string, error) {
			return map[string]string{"message": in.Body.Message}, nil
		},
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/limited-body",
		bytes.NewBufferString(`{"message":"too large"}`),
	)
	req.ContentLength = -1
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode problem response: %v", err)
	}
	if body.Title != "Payload Too Large" || body.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected problem response: %+v", body)
	}
}
