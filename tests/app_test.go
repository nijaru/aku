package aku_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

func TestApp_Middleware(t *testing.T) {
	app := aku.New()

	// Add middleware that sets a response header
	app.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Global-Middleware", "active")
			next.ServeHTTP(w, r)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/any-path", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Header().Get("X-Global-Middleware") != "active" {
		t.Errorf(
			"expected X-Global-Middleware header, got %q",
			rr.Header().Get("X-Global-Middleware"),
		)
	}
}
