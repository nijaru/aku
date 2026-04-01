package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

func TestGroup(t *testing.T) {
	app := aku.New()
	var order []string

	v1 := app.Group("/v1", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "v1")
			next.ServeHTTP(w, r)
		})
	})

	users := v1.Group("/users", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "users")
			next.ServeHTTP(w, r)
		})
	})

	aku.Get(users, "/{id}", func(ctx context.Context, in struct {
		Path struct {
			ID string `path:"id"`
		}
	},
	) (map[string]string, error) {
		order = append(order, "handler:"+in.Path.ID)
		return map[string]string{"id": in.Path.ID}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/users/123", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	expectedOrder := []string{"v1", "users", "handler:123"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("expected order %v, got %v", expectedOrder, order)
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("at index %d: expected %s, got %s", i, v, order[i])
		}
	}
}
