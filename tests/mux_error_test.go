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

		var prob problem.Details
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

		var prob problem.Details
		if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
			t.Fatal(err)
		}

		if prob.Status != http.StatusMethodNotAllowed {
			t.Errorf("Expected problem status 405, got %d", prob.Status)
		}
	})
}

func TestApp_Custom404(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/custom-404", func(ctx context.Context, in struct{}) (string, error) {
		return "", problem.Problemf(http.StatusNotFound, "Custom 404", "This is a custom 404")
	})

	req := httptest.NewRequest(http.MethodGet, "/custom-404", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var prob problem.Details
	if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
		t.Fatal(err)
	}

	if prob.Title != "Custom 404" {
		t.Errorf("Expected custom problem title, got %q", prob.Title)
	}
}
