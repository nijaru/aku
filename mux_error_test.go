package aku_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

func TestApp_MuxErrors(t *testing.T) {
	app := aku.New()

	t.Run("404 Not Found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/non-existent", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}

		if w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("Expected content-type application/problem+json, got %q", w.Header().Get("Content-Length"))
		}

		var prob aku.Problem
		if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
			t.Fatal(err)
		}

		if prob.Status != http.StatusNotFound {
			t.Errorf("Expected problem status 404, got %d", prob.Status)
		}
	})

	t.Run("405 Method Not Allowed", func(t *testing.T) {
		aku.Post(app, "/only-post", func(ctx context.Context, in struct{}) (string, error) {
			return "ok", nil
		})

		req := httptest.NewRequest(http.MethodGet, "/only-post", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", w.Code)
		}

		if w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("Expected content-type application/problem+json, got %q", w.Header().Get("Content-Type"))
		}

		var prob aku.Problem
		if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
			t.Fatal(err)
		}

		if prob.Status != http.StatusMethodNotAllowed {
			t.Errorf("Expected problem status 405, got %d", prob.Status)
		}
	})
}
