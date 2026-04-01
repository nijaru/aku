package aku_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/problem"
)

type MyInput struct {
	Path struct {
		ID int `path:"id"`
	}
}
type MyOutput struct {
	ID int `json:"id"`
}

func myHandler(ctx context.Context, in MyInput) (MyOutput, error) {
	return MyOutput{ID: in.Path.ID}, nil
}

func TestRegister(t *testing.T) {
	app := aku.New()

	err := aku.Get(app, "/test/{id}", myHandler, aku.WithStatus(http.StatusCreated))
	if err != nil {
		t.Fatalf("expected nil error on register, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test/123", nil)
	// Go 1.22 path values mock for manual testing without a real ServeHTTP route match,
	// but here we are using the real mux via app.ServeHTTP
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rr.Code)
	}

	var out MyOutput
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if out.ID != 123 {
		t.Errorf("expected ID 123, got %d", out.ID)
	}
}

func TestRegister_NoContent(t *testing.T) {
	app := aku.New()

	aku.Post(app, "/test", func(ctx context.Context, in MyInput) (any, error) {
		return nil, nil
	}, aku.WithStatus(http.StatusNoContent))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content, got %d", rr.Code)
	}

	if rr.Body.Len() > 0 {
		t.Errorf("expected empty body, got %q", rr.Body.String())
	}
}

func TestRegister_Error(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/test/{id}", myHandler)

	req := httptest.NewRequest(http.MethodGet, "/test/abc", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity for coercion error, got %d", rr.Code)
	}

	var prob problem.Details
	if err := json.Unmarshal(rr.Body.Bytes(), &prob); err != nil {
		t.Fatalf("failed to unmarshal problem: %v", err)
	}

	if len(prob.InvalidParams) == 0 {
		t.Fatal("expected invalid_params in problem response")
	}

	if prob.InvalidParams[0].Name != "id" || prob.InvalidParams[0].In != "path" {
		t.Errorf("unexpected invalid_param: %+v", prob.InvalidParams[0])
	}
}

func TestMiddleware(t *testing.T) {
	app := aku.New()
	var order []string

	// Global middleware
	app.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "global")
			next.ServeHTTP(w, r)
		})
	})

	// Route-specific middleware
	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "route")
			next.ServeHTTP(w, r)
		})
	}

	aku.Get(app, "/test", func(ctx context.Context, in any) (any, error) {
		order = append(order, "handler")
		return map[string]string{"status": "ok"}, nil
	}, aku.WithMiddleware(routeMW))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	expectedOrder := []string{"global", "route", "handler"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("expected order length %d, got %d", len(expectedOrder), len(order))
	}

	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("at index %d: expected %s, got %s", i, v, order[i])
		}
	}
}
