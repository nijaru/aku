package aku_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRegisterRejectsUnsupportedInputTypes(t *testing.T) {
	app := aku.New()
	type In struct {
		Query struct {
			Value chan int `query:"value"`
		}
	}

	err := aku.Get(app, "/unsupported", func(ctx context.Context, in In) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil {
		t.Fatal("expected unsupported tagged type to fail at registration")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unexpected registration error: %v", err)
	}

	type BadSection struct {
		Query string
	}
	err = aku.Get(app, "/bad-section", func(ctx context.Context, in BadSection) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "Query section must be a struct") {
		t.Fatalf("expected invalid Query section to fail at registration, got %v", err)
	}
}

func TestRegisterRejectsInvalidAuthDeclaration(t *testing.T) {
	app := aku.New()
	type In struct {
		Auth struct {
			Key string `auth:"apikey:heder:X-API-Key"`
		}
	}

	err := aku.Get(app, "/invalid-auth", func(ctx context.Context, in In) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "invalid API key declaration") {
		t.Fatalf("expected invalid auth declaration error, got %v", err)
	}

	type BadBearerTag struct {
		Auth struct {
			Token string `auth:"bearer:token"`
		}
	}
	err = aku.Get(
		app,
		"/invalid-bearer",
		func(ctx context.Context, in BadBearerTag) (struct{}, error) {
			return struct{}{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported authentication declaration") {
		t.Fatalf("expected invalid bearer declaration error, got %v", err)
	}
}

func TestRegisterRejectsBodyAndForm(t *testing.T) {
	app := aku.New()
	type In struct {
		Body struct {
			Name string `json:"name"`
		}
		Form struct {
			Value string `form:"value"`
		}
	}

	err := aku.Post(app, "/both", func(ctx context.Context, in In) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "both Body and Form") {
		t.Fatalf("expected Body/Form conflict error, got %v", err)
	}
}

func TestRegisterRejectsMismatchedPathBinding(t *testing.T) {
	app := aku.New()
	type In struct {
		Path struct {
			ID string `path:"other"`
		}
	}

	err := aku.Get(app, "/users/{id}", func(ctx context.Context, in In) (struct{}, error) {
		return struct{}{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "no matching path binding") {
		t.Fatalf("expected path binding error, got %v", err)
	}
}

func TestRegisterRejectsConflictingRoutes(t *testing.T) {
	app := aku.New()
	handler := func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, nil
	}

	if err := aku.Get(app, "/conflict", handler); err != nil {
		t.Fatalf("unexpected first registration error: %v", err)
	}
	if err := aku.Get(app, "/conflict", handler); err == nil {
		t.Fatal("expected conflicting route registration to return an error")
	}
}

func TestRoutesReturnsMetadataSnapshot(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/snapshot", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	}, aku.WithTags("original"))

	routes := app.Routes()
	routes[0].Summary = "mutated"
	routes[0].Tags[0] = "mutated"

	doc := app.OpenAPIDocument("Snapshot API", "1.0.0")
	operation := doc.Paths["/snapshot"]["get"]
	if operation.Summary == "mutated" || operation.Tags[0] == "mutated" {
		t.Fatal("mutating Routes() result changed application metadata")
	}
}

func TestHandleWithoutMetadataIsExcludedFromOpenAPI(t *testing.T) {
	app := aku.New()
	app.Handle(
		http.MethodGet,
		"/raw",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		nil,
	)

	doc := app.OpenAPIDocument("Raw API", "1.0.0")
	if _, ok := doc.Paths["/raw"]; ok {
		t.Fatal("expected raw route without metadata to be excluded from OpenAPI")
	}
}

func TestRegistrationMethodsReturnErrors(t *testing.T) {
	app := aku.New()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	if err := app.HandleHTTP(http.MethodGet, "/raw", handler); err != nil {
		t.Fatalf("unexpected raw handler registration error: %v", err)
	}
	if err := app.HandleHTTP(http.MethodGet, "/raw", handler); err == nil {
		t.Fatal("expected duplicate raw handler registration to return an error")
	}

	if err := app.Metrics("/metrics", handler); err != nil {
		t.Fatalf("unexpected metrics registration error: %v", err)
	}
	if err := app.OpenAPI("/openapi.json", "API", "1.0.0"); err != nil {
		t.Fatalf("unexpected OpenAPI registration error: %v", err)
	}
	if err := app.OpenAPI("/openapi.json", "API", "1.0.0"); err == nil {
		t.Fatal("expected duplicate OpenAPI registration to return an error")
	}
	if err := app.SwaggerUI("/docs", "/openapi.json"); err != nil {
		t.Fatalf("unexpected Swagger UI registration error: %v", err)
	}
	if err := app.RedocUI("/redoc", "/openapi.json"); err != nil {
		t.Fatalf("unexpected Redoc registration error: %v", err)
	}
	if err := app.StaticFS("/assets", nil); err == nil {
		t.Fatal("expected nil static file system to return an error")
	}
	if err := app.HandleHTTP(http.MethodGet, "/partial", handler); err != nil {
		t.Fatalf("unexpected setup route registration error: %v", err)
	}
	if err := app.StaticFS("/partial", http.Dir(".")); err == nil {
		t.Fatal("expected conflicting static route registration to return an error")
	}
	partial := httptest.NewRecorder()
	app.ServeHTTP(partial, httptest.NewRequest(http.MethodGet, "/partial/test", nil))
	if partial.Code != http.StatusNotFound {
		t.Fatalf("failed static registration partially changed the mux: got %d", partial.Code)
	}

	group := app.Group("/v1")
	if err := group.HandleHTTP(http.MethodGet, "/raw", handler); err != nil {
		t.Fatalf("unexpected group handler registration error: %v", err)
	}
	if err := group.Metrics("/metrics", handler); err != nil {
		t.Fatalf("unexpected group metrics registration error: %v", err)
	}
}

func TestFlushCommitsSuccessfulStatus(t *testing.T) {
	app := aku.New()
	app.HandleHTTP(
		http.MethodGet,
		"/flush",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("expected App to preserve http.Flusher")
				return
			}
			flusher.Flush()
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/flush", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected Flush to commit 200, got %d", rec.Code)
	}
}

func TestRegister_NoContent(t *testing.T) {
	app := aku.New()

	aku.Post(app, "/test", func(ctx context.Context, in struct{}) (any, error) {
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

func TestRegister_RejectsTrailingJSONBody(t *testing.T) {
	app := aku.New()

	type In struct {
		Body struct {
			Name string `json:"name"`
		}
	}

	aku.Post(app, "/body", func(ctx context.Context, in In) (map[string]string, error) {
		return map[string]string{"name": in.Body.Name}, nil
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/body",
		strings.NewReader(`{"name":"aku"}{"name":"extra"}`),
	)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for trailing JSON body, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlerErrorDoesNotLeakInternalDetail(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/failure", func(ctx context.Context, in struct{}) (struct{}, error) {
		return struct{}{}, errors.New("database password: super-secret")
	})

	req := httptest.NewRequest(http.MethodGet, "/failure", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "super-secret") {
		t.Fatalf("handler error detail leaked in response: %s", rec.Body.String())
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

func TestHandleHTTP(t *testing.T) {
	app := aku.New()
	var order []string

	app.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "global")
			next.ServeHTTP(w, r)
		})
	})

	routeMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "route")
			next.ServeHTTP(w, r)
		})
	}

	app.HandleHTTP(
		http.MethodGet,
		"/metrics",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusCreated)
		}),
		aku.WithMiddleware(routeMW),
		aku.WithStatus(http.StatusCreated),
		aku.WithSummary("Metrics"),
	)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rr.Code)
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

	doc := app.OpenAPIDocument("Test API", "1.0.0")
	path, ok := doc.Paths["/metrics"]
	if !ok {
		t.Fatal("expected /metrics path in OpenAPI document")
	}

	op := path["get"]
	if op == nil {
		t.Fatal("expected GET operation for /metrics")
	}
	if op.Summary != "Metrics" {
		t.Fatalf("expected summary Metrics, got %q", op.Summary)
	}
	if _, ok := op.Responses["201"]; !ok {
		t.Fatalf("expected 201 response in OpenAPI, got %+v", op.Responses)
	}
}
