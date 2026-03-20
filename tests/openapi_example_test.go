package aku_test

import (
	"context"
	"testing"

	"github.com/nijaru/aku"
)

func TestOpenAPI_Examples(t *testing.T) {
	app := aku.New()

	type ExampleRequest struct {
		Query struct {
			Search string `query:"search" example:"gopher"`
		}
		Body struct {
			Name string `json:"name" example:"Nick"`
			Age  int    `json:"age" example:"30"`
		}
	}

	aku.Post(app, "/example", func(ctx context.Context, in ExampleRequest) (struct{}, error) {
		return struct{}{}, nil
	})

	doc := app.OpenAPIDocument("Example API", "1.0.0")
	path := doc.Paths["/example"]["post"]

	// Check Query parameter example
	searchParam := path.Parameters[0]
	if searchParam.Schema.Example != "gopher" {
		t.Errorf("expected search example 'gopher', got %v", searchParam.Schema.Example)
	}

	// Check Body schema examples
	bodyContent := path.RequestBody.Content["application/json"]
	bodySchema := bodyContent.Schema

	// Since we use reflectToSchema, Body points to a named or anonymous struct.
	// In this case it's an anonymous struct because field.Type was used directly in compiler.go:96
	// Wait, compiler.go:96: schema.Body = field.Type. field.Type is ExampleRequest.Body (anonymous struct).

	nameProp := bodySchema.Properties["name"]
	if nameProp.Example != "Nick" {
		t.Errorf("expected name example 'Nick', got %v", nameProp.Example)
	}

	ageProp := bodySchema.Properties["age"]
	if ageProp.Example != "30" {
		t.Errorf("expected age example '30', got %v", ageProp.Example)
	}
}
