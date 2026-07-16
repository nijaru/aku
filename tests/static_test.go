package aku_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nijaru/aku"
)

func TestRouter_Static(t *testing.T) {
	app := aku.New()

	// Create a temporary directory with a file
	tmpDir, err := os.MkdirTemp("", "aku-static-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := "hello static"
	err = os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	app.Static("/static", tmpDir)

	t.Run("Serves file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		if w.Body.String() != content {
			t.Errorf("Expected body %q, got %q", content, w.Body.String())
		}
	})

	t.Run("404 for non-existent file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/missing.txt", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
		// Should be redacted by our interceptor to a Problem
		if w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("Expected problem+json, got %q", w.Header().Get("Content-Type"))
		}
	})

	t.Run("Redirects exact prefix", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != http.StatusMovedPermanently {
			t.Errorf("Expected status 301, got %d", w.Code)
		}
		if w.Header().Get("Location") != "/static/" {
			t.Errorf("Expected location /static/, got %q", w.Header().Get("Location"))
		}
	})
}

func TestGroup_Static(t *testing.T) {
	app := aku.New()
	var middlewareCalls int
	v1 := app.Group("/v1", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalls++
			next.ServeHTTP(w, r)
		})
	})

	tmpDir, _ := os.MkdirTemp("", "aku-group-static-test")
	defer os.RemoveAll(tmpDir)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("v1 content"), 0o644)

	v1.Static("/assets", tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/assets/test.txt", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "v1 content" {
		t.Errorf("Expected 'v1 content', got %q", w.Body.String())
	}
	if middlewareCalls != 1 {
		t.Fatalf("expected group middleware to wrap static route, got %d calls", middlewareCalls)
	}
}

func TestStaticPrefixRedirectUsesRouteMiddleware(t *testing.T) {
	app := aku.New()
	tmpDir, err := os.MkdirTemp("", "aku-static-redirect-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	var calls int
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			next.ServeHTTP(w, r)
		})
	}
	if err := app.Static("/private", tmpDir, aku.WithMiddleware(middleware)); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/private", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect, got %d", rec.Code)
	}
	if calls != 1 {
		t.Fatalf("expected route middleware on exact-prefix redirect, got %d calls", calls)
	}
}
