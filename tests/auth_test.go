package aku_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
)

// Bearer-only input struct — Token is extracted from Authorization header.
type BearerInput struct {
	Auth struct {
		Token auth.Bearer
	}
}

func protectedHandler(ctx context.Context, in BearerInput) (map[string]string, error) {
	return map[string]string{"token": string(in.Auth.Token)}, nil
}

// APIKey input — extracted from custom header.
type APIKeyInput struct {
	Auth struct {
		Key auth.APIKey `auth:"apikey:header:X-API-Key"`
	}
}

func apiKeyHandler(ctx context.Context, in APIKeyInput) (map[string]string, error) {
	return map[string]string{"key": string(in.Auth.Key)}, nil
}

func setupAuthTest(t *testing.T) *aku.App {
	t.Helper()
	app := aku.New()
	aku.Get(app, "/protected", protectedHandler)
	aku.Get(app, "/api-key", apiKeyHandler)
	return app
}

func TestBearerExtraction_Success(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token-123")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["token"] != "my-secret-token-123" {
		t.Fatalf("expected token 'my-secret-token-123', got %q", resp["token"])
	}
}

func TestBearerExtraction_Missing(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBearerExtraction_WrongScheme(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong auth scheme, got %d", rec.Code)
	}
}

func TestBearerExtraction_EmptyToken(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty token, got %d", rec.Code)
	}
}

func TestAPIKeyExtraction_Success(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api-key", nil)
	req.Header.Set("X-API-Key", "api-key-value-456")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["key"] != "api-key-value-456" {
		t.Fatalf("expected key 'api-key-value-456', got %q", resp["key"])
	}
}

func TestAPIKeyExtraction_Missing(t *testing.T) {
	app := setupAuthTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api-key", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthOpenAPI_SecuritySchemes(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/protected", protectedHandler)

	app.OpenAPI("/openapi.json", "Auth Test API", "1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}

	components, ok := doc["components"].(map[string]interface{})
	if !ok {
		t.Fatal("expected components in OpenAPI doc")
	}

	schemes, ok := components["securitySchemes"].(map[string]interface{})
	if !ok {
		t.Fatal("expected securitySchemes in OpenAPI components")
	}

	if len(schemes) == 0 {
		t.Fatal("expected at least one security scheme, got none")
	}
}

func TestRequireBearerMiddleware(t *testing.T) {
	app := aku.New()
	app.Use(auth.RequireBearer())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", rec2.Code)
	}
}

func TestRequireAPIKeyMiddleware(t *testing.T) {
	app := aku.New()
	app.Use(auth.RequireAPIKey("X-API-Key"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("X-API-Key", "secret-key")
	rec2 := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid API key, got %d", rec2.Code)
	}
}
