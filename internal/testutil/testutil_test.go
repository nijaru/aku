package testutil_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
)

type User struct {
	ID   int    `json:"id" validate:"required"`
	Name string `json:"name" validate:"required"`
}

func TestTester_Fixes(t *testing.T) {
	app := aku.New()

	aku.Post(app, "/echo", func(ctx context.Context, in struct {
		Body User
	}) (User, error) {
		return in.Body, nil
	})

	at := testutil.Test(t, app)

	t.Run("Body exhaustion fix", func(t *testing.T) {
		req := at.Post("/echo").WithJSON(User{ID: 1, Name: "Alice"})

		// First call
		req.ExpectStatus(http.StatusOK).ExpectJSON(User{ID: 1, Name: "Alice"})

		// Second call with same request builder should work now
		req.ExpectStatus(http.StatusOK).ExpectJSON(User{ID: 1, Name: "Alice"})
	})

	t.Run("Nil JSON expectation fix", func(t *testing.T) {
		aku.Get(app, "/empty", func(ctx context.Context, in struct{}) (any, error) {
			return nil, nil
		}, aku.WithStatus(http.StatusNoContent))

		at.Get("/empty").
			ExpectStatus(http.StatusNoContent).
			ExpectJSON(nil)
	})

	t.Run("Numeric comparison fix", func(t *testing.T) {
		aku.Get(app, "/number", func(ctx context.Context, in struct{}) (map[string]any, error) {
			return map[string]any{"val": 123}, nil
		})

		// 123 in map will be float64 after unmarshal, but our fix should handle it
		at.Get("/number").
			ExpectStatus(http.StatusOK).
			ExpectJSON(map[string]any{"val": 123})
	})
}

func TestTester_WithBody(t *testing.T) {
	app := aku.New()
	aku.Post(app, "/body", func(ctx context.Context, in struct {
		Body map[string]string
	}) (map[string]string, error) {
		return in.Body, nil
	})

	at := testutil.Test(t, app)
	at.Post("/body").
		WithBody(strings.NewReader(`{"foo":"bar"}`)).
		ExpectStatus(http.StatusOK).
		ExpectJSON(map[string]string{"foo": "bar"})
}
