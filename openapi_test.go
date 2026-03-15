package aku_test

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/nijaru/aku"
)

type UserRequest struct {
	Path struct {
		ID string `path:"id"`
	}
	Query struct {
		Verbose bool `query:"verbose"`
	}
}

type UserResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func GetUser(ctx context.Context, in UserRequest) (*UserResponse, error) {
	return &UserResponse{ID: in.Path.ID, Name: "aku"}, nil
}

func TestOpenAPI(t *testing.T) {
	app := aku.New()
	aku.Get(app, "/users/{id}", GetUser, 
		aku.WithSummary("Get a user"),
		aku.WithDescription("Returns a user by their ID"),
		aku.WithTags("Users"),
	)

	doc := app.OpenAPI("My API", "1.0.0")
	if doc.Info.Title != "My API" {
		t.Errorf("expected title My API, got %s", doc.Info.Title)
	}

	path, ok := doc.Paths["/users/{id}"]
	if !ok {
		t.Fatal("expected path /users/{id} to be present in OpenAPI doc")
	}

	op := path["get"]
	if op == nil {
		t.Fatal("expected get operation to be present")
	}

	if op.Summary != "Get a user" {
		t.Errorf("expected summary 'Get a user', got %q", op.Summary)
	}

	if len(op.Tags) != 1 || op.Tags[0] != "Users" {
		t.Errorf("expected tags [Users], got %v", op.Tags)
	}

	// Verify parameter extraction in OpenAPI
	if len(op.Parameters) != 2 {
		t.Errorf("expected 2 parameters (path + query), got %d", len(op.Parameters))
	}

	// Check JSON output
	data, err := doc.JSON()
	if err != nil {
		t.Fatalf("failed to generate JSON: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal generated JSON: %v", err)
	}
	
	if raw["openapi"] != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %v", raw["openapi"])
	}

	// Verify components
	components := raw["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	if _, ok := schemas["github.com.nijaru.aku_test.UserResponse"]; !ok {
		t.Error("expected UserResponse in components/schemas")
	}
}

func TestOpenAPI_Advanced(t *testing.T) {
	app := aku.New()

	type AdvancedRequest struct {
		Query struct {
			Age int `query:"age" validate:"min=18,max=120"`
		}
		Form struct {
			Name   string                `form:"name" validate:"required"`
			Avatar *multipart.FileHeader `form:"avatar"`
		}
	}

	aku.Post(app, "/advanced", func(ctx context.Context, in AdvancedRequest) (aku.SSE, error) {
		return aku.SSE{}, nil
	})

	doc := app.OpenAPI("Advanced API", "1.1.0")
	path := doc.Paths["/advanced"]["post"]

	// Verify validation on Query
	ageParam := path.Parameters[0]
	if ageParam.Name != "age" {
		t.Fatalf("expected age param, got %s", ageParam.Name)
	}
	if *ageParam.Schema.Minimum != 18 || *ageParam.Schema.Maximum != 120 {
		t.Errorf("expected age validation 18-120, got min=%v, max=%v", *ageParam.Schema.Minimum, *ageParam.Schema.Maximum)
	}

	// Verify Form in RequestBody
	formBody := path.RequestBody.Content["multipart/form-data"]
	if formBody.Schema.Type != "object" {
		t.Errorf("expected object form body, got %s", formBody.Schema.Type)
	}
	if formBody.Schema.Properties["avatar"].Format != "binary" {
		t.Errorf("expected binary avatar, got format %s", formBody.Schema.Properties["avatar"].Format)
	}
	if len(formBody.Schema.Required) != 1 || formBody.Schema.Required[0] != "name" {
		t.Errorf("expected name to be required in form, got %v", formBody.Schema.Required)
	}

	// Verify SSE Response
	res := path.Responses["200"]
	if _, ok := res.Content["text/event-stream"]; !ok {
		t.Errorf("expected text/event-stream response, got %v", res.Content)
	}
}

