package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nijaru/aku"
)

type User struct {
	ID   string
	Name string
}

type CtxSection struct {
	User *User `ctx:"user"`
}

type CtxInput struct {
	Ctx CtxSection
}

func TestContextInjection(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/context", func(ctx context.Context, in CtxInput) (string, error) {
		if in.Ctx.User == nil {
			return "no user", nil
		}
		return in.Ctx.User.Name, nil
	})

	t.Run("with user in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/context", nil)
		req = req.WithContext(context.WithValue(req.Context(), "user", &User{ID: "1", Name: "John"}))
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "John") {
			t.Errorf("Expected body to contain 'John', got %q", body)
		}
	})

	t.Run("without user in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/context", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "no user") {
			t.Errorf("Expected body to contain 'no user', got %q", body)
		}
	})
}
