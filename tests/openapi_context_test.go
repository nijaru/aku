package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nijaru/aku"
)

func TestOpenAPIContextLeaking(t *testing.T) {
	app := aku.New()

	type ContextIn struct {
		Ctx struct {
			User string `ctx:"user"`
		}
	}

	aku.Get(app, "/context", func(ctx context.Context, in ContextIn) (string, error) {
		return in.Ctx.User, nil
	})

	doc := app.OpenAPIDocument("Context API", "1.0.0")
	path, ok := doc.Paths["/context"]
	if !ok {
		t.Fatal("expected /context path")
	}
	op := path["get"]

	for _, p := range op.Parameters {
		if p.In == "context" {
			t.Errorf("leak: context parameter %q found in OpenAPI spec", p.Name)
		}
	}
}

func TestContextEnforcement(t *testing.T) {
	app := aku.New()

	type User struct {
		Name string
	}

	type RequiredIn struct {
		Ctx struct {
			User *User `ctx:"user"`
		}
	}

	type OptionalIn struct {
		Ctx struct {
			User *User `ctx:"user" aku:"optional"`
		}
	}

	aku.Get(app, "/required", func(ctx context.Context, in RequiredIn) (string, error) {
		return in.Ctx.User.Name, nil
	})

	aku.Get(app, "/optional", func(ctx context.Context, in OptionalIn) (string, error) {
		if in.Ctx.User == nil {
			return "none", nil
		}
		return in.Ctx.User.Name, nil
	})

	t.Run("required missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/required", nil)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		// Aku uses 422 for extraction/validation errors
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for missing required context, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "missing required context value") {
			t.Errorf("expected error message about missing context, got %q", w.Body.String())
		}
	})

	t.Run("optional missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/optional", nil)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for missing optional context, got %d", w.Code)
		}
		// "none" is encoded as JSON string "\"none\"\n"
		if !strings.Contains(w.Body.String(), "none") {
			t.Errorf("expected 'none', got %q", w.Body.String())
		}
	})

	t.Run("type mismatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/required", nil)
		// Inject a string instead of *User
		req = req.WithContext(
			context.WithValue(req.Context(), aku.ContextKey("user"), "not-a-user"),
		)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422 for type mismatch, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "expected *aku_test.User") {
			t.Errorf("expected error message about type mismatch, got %q", w.Body.String())
		}
	})
}
