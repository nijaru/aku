package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReferenceAppSurface(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("embedded web page", func(t *testing.T) {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Aku reference app") {
			t.Fatalf("unexpected embedded page response: %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("operational endpoints", func(t *testing.T) {
		for _, path := range []string{"/health", "/ready", "/api/version"} {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected %s to return 200, got %d: %s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("typed route and generated contract", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/items/42",
			strings.NewReader(`{"name":"Ada"}`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected typed route to return 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var item map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		if item["id"] != "42" || item["name"] != "Ada" {
			t.Fatalf("unexpected typed response: %+v", item)
		}

		spec := httptest.NewRecorder()
		app.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
		if spec.Code != http.StatusOK ||
			!bytes.Contains(spec.Body.Bytes(), []byte(`/api/items/{id}`)) {
			t.Fatalf("generated contract omitted typed route: %d %s", spec.Code, spec.Body.String())
		}
		var document struct {
			Components struct {
				SecuritySchemes map[string]any `json:"securitySchemes"`
			} `json:"components"`
		}
		if err := json.Unmarshal(spec.Body.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Components.SecuritySchemes) != 1 {
			t.Fatalf(
				"expected one inferred security scheme, got %v",
				document.Components.SecuritySchemes,
			)
		}
		if _, ok := document.Components.SecuritySchemes["Token"]; !ok {
			t.Fatalf(
				"expected inferred Token security scheme, got %v",
				document.Components.SecuritySchemes,
			)
		}
	})
}
