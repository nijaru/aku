package aku_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
)

type ValidationRequest struct {
	Query struct {
		Age int `query:"age" validate:"required,min=18" msg:"You must be at least 18 years old"`
	}
	Body struct {
		Email string `json:"email" validate:"required,email" msg:"Please provide a valid corporate email address"`
	}
}

func TestValidationMessages(t *testing.T) {
	v := validator.New()
	app := aku.New(aku.WithValidator(v))

	aku.Post(app, "/validate", func(ctx context.Context, in ValidationRequest) (string, error) {
		return "ok", nil
	})

	t.Run("Custom query message", func(t *testing.T) {
		testutil.Test(t, app).
			Post("/validate?age=10").
			WithJSON(map[string]string{"email": "test@example.com"}).
			ExpectStatus(http.StatusUnprocessableEntity).
			ExpectJSONFunc(func(m map[string]any) error {
				params, ok := m["invalid_params"].([]any)
				if !ok {
					t.Fatalf("expected invalid_params, got %+v", m)
				}
				found := false
				for _, p := range params {
					param := p.(map[string]any)
					if param["name"] == "age" {
						found = true
						if param["reason"] != "You must be at least 18 years old" {
							t.Errorf("expected custom message, got %v", param["reason"])
						}
					}
				}
				if !found {
					t.Error("expected validation error for 'age'")
				}
				return nil
			})
	})

	t.Run("Custom body message", func(t *testing.T) {
		testutil.Test(t, app).
			Post("/validate?age=20").
			WithJSON(map[string]string{"email": "invalid-email"}).
			ExpectStatus(http.StatusUnprocessableEntity).
			ExpectJSONFunc(func(m map[string]any) error {
				params := m["invalid_params"].([]any)
				found := false
				for _, p := range params {
					param := p.(map[string]any)
					if param["name"] == "email" {
						found = true
						if param["reason"] != "Please provide a valid corporate email address" {
							t.Errorf("expected custom message, got %v", param["reason"])
						}
					}
				}
				if !found {
					t.Error("expected validation error for 'email'")
				}
				return nil
			})
	})

	t.Run("Default message fallback", func(t *testing.T) {
		type DefaultReq struct {
			Query struct {
				Name string `query:"name" validate:"required"`
			}
		}
		app2 := aku.New(aku.WithValidator(v))
		aku.Get(app2, "/default", func(ctx context.Context, in DefaultReq) (string, error) {
			return "ok", nil
		})

		testutil.Test(t, app2).
			Get("/default").
			ExpectStatus(http.StatusUnprocessableEntity).
			ExpectJSONFunc(func(m map[string]any) error {
				params := m["invalid_params"].([]any)
				param := params[0].(map[string]any)
				if param["reason"] != "is required" {
					t.Errorf("expected 'is required', got %v", param["reason"])
				}
				return nil
			})
	})
}
