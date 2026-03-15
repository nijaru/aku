package aku_test

import (
	"context"
	"encoding/json"
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
	if _, ok := schemas["UserResponse"]; !ok {
		t.Error("expected UserResponse in components/schemas")
	}
}
