package aku_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
	"github.com/nijaru/aku/file"
	"github.com/nijaru/aku/middleware"
)

type acceptanceItemInput struct {
	Auth struct {
		Token auth.Bearer
	}
	Path struct {
		ID string `path:"id"`
	}
	Query struct {
		Echo string `query:"echo" aku:"optional"`
	}
	Body struct {
		Name string `json:"name" validate:"required"`
	}
}

type acceptanceUploadInput struct {
	Form struct {
		Name  string                `form:"name"`
		Photo *multipart.FileHeader `form:"photo" aku:"required"`
	}
}

type acceptanceWebhookInput struct {
	Header struct {
		Signature string `header:"X-Signature"`
	}
	Body struct {
		Event string `json:"event"`
	}
}

func TestAcceptance_ApplicationSurface(t *testing.T) {
	app := aku.New(
		aku.WithValidator(validator.New()),
		aku.WithGlobalMiddleware(
			middleware.BodySizeLimit(middleware.BodySizeLimitConfig{MaxBodyBytes: 1 << 20}),
		),
	)

	if err := aku.Post(
		app,
		"/api/items/{id}",
		func(ctx context.Context, in acceptanceItemInput) (map[string]string, error) {
			return map[string]string{
				"id":    in.Path.ID,
				"name":  in.Body.Name,
				"echo":  in.Query.Echo,
				"token": string(in.Auth.Token),
			}, nil
		},
		aku.WithStatus(http.StatusCreated),
	); err != nil {
		t.Fatal(err)
	}

	if err := aku.Post(
		app,
		"/api/uploads",
		func(ctx context.Context, in acceptanceUploadInput) (string, error) {
			uploaded := file.File{Header: in.Form.Photo}
			if err := uploaded.Validate(1<<20, "text/plain; charset=utf-8"); err != nil {
				return "", err
			}
			return in.Form.Name, nil
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := aku.Post(
		app,
		"/webhooks/provider",
		func(ctx context.Context, in acceptanceWebhookInput) (map[string]string, error) {
			return map[string]string{
				"event":     in.Body.Event,
				"signature": in.Header.Signature,
			}, nil
		},
	); err != nil {
		t.Fatal(err)
	}

	checks := middleware.NewHealthChecker()
	checks.Add("database", middleware.HealthyCheck)
	app.Use(checks.Middleware)

	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<html>app</html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Static("/web", webRoot, aku.WithSPA())
	app.OpenAPI("/openapi.json", "Acceptance API", "1.0.0")

	t.Run("typed API auth and validation", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/items/42?echo=hello",
			strings.NewReader(`{"name":"Ada"}`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["id"] != "42" || got["name"] != "Ada" || got["echo"] != "hello" ||
			got["token"] != "test-token" {
			t.Fatalf("unexpected API response: %+v", got)
		}

		missingAuth := httptest.NewRecorder()
		app.ServeHTTP(
			missingAuth,
			httptest.NewRequest(
				http.MethodPost,
				"/api/items/42",
				strings.NewReader(`{"name":"Ada"}`),
			),
		)
		if missingAuth.Code != http.StatusUnauthorized ||
			missingAuth.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Fatalf(
				"expected bearer-protected 401, got %d with challenge %q",
				missingAuth.Code,
				missingAuth.Header().Get("WWW-Authenticate"),
			)
		}

		invalidBody := httptest.NewRecorder()
		invalidReq := httptest.NewRequest(
			http.MethodPost,
			"/api/items/42",
			strings.NewReader(`{"name":""}`),
		)
		invalidReq.Header.Set("Authorization", "Bearer test-token")
		invalidReq.Header.Set("Content-Type", "application/json")
		app.ServeHTTP(invalidBody, invalidReq)
		if invalidBody.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected validation 422, got %d", invalidBody.Code)
		}
	})

	t.Run("multipart upload", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("name", "notes"); err != nil {
			t.Fatal(err)
		}
		part, err := writer.CreateFormFile("photo", "notes.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("plain text")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/uploads", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `"notes"` {
			t.Fatalf("unexpected upload response: %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("webhook-style request", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPost,
			"/webhooks/provider",
			strings.NewReader(`{"event":"payment.completed"}`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Signature", "sig-123")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected webhook 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var got map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["event"] != "payment.completed" || got["signature"] != "sig-123" {
			t.Fatalf("unexpected webhook response: %+v", got)
		}
	})

	t.Run("health, SPA, and OpenAPI", func(t *testing.T) {
		for _, path := range []string{"/health", "/ready"} {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected %s to return 200, got %d", path, rec.Code)
			}
		}

		spa := httptest.NewRecorder()
		app.ServeHTTP(spa, httptest.NewRequest(http.MethodGet, "/web/dashboard", nil))
		if spa.Code != http.StatusOK || spa.Body.String() != "<html>app</html>\n" {
			t.Fatalf("unexpected SPA response: %d %q", spa.Code, spa.Body.String())
		}

		doc := httptest.NewRecorder()
		app.ServeHTTP(doc, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
		if doc.Code != http.StatusOK ||
			!bytes.Contains(doc.Body.Bytes(), []byte(`/api/items/{id}`)) {
			t.Fatalf("OpenAPI document did not include typed route: %d", doc.Code)
		}
	})
}
