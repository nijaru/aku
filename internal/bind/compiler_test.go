package bind_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nijaru/aku/internal/bind"
)

type ValidationRequest struct {
	Body struct {
		Age int `json:"age"`
	}
}

func (v *ValidationRequest) Validate() error {
	if v.Body.Age < 18 {
		return fmt.Errorf("age must be at least 18")
	}
	return nil
}

func TestCompiler_OptionalFields(t *testing.T) {
	type OptionalRequest struct {
		Query struct {
			Page *int `query:"page"`
		}
		Header struct {
			TraceID *string `header:"X-Trace-Id"`
		}
	}

	extractor, _ := bind.Compiler[OptionalRequest]()

	t.Run("present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=2", nil)
		req.Header.Set("X-Trace-Id", "trace-123")

		var in OptionalRequest
		v := reflect.ValueOf(&in).Elem()

		if err := extractor(context.Background(), req, v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if in.Query.Page == nil || *in.Query.Page != 2 {
			t.Errorf("expected Query.Page=2, got %v", in.Query.Page)
		}
		if in.Header.TraceID == nil || *in.Header.TraceID != "trace-123" {
			t.Errorf("expected Header.TraceID=trace-123, got %v", in.Header.TraceID)
		}
	})

	t.Run("absent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		var in OptionalRequest
		v := reflect.ValueOf(&in).Elem()

		if err := extractor(context.Background(), req, v); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if in.Query.Page != nil {
			t.Errorf("expected Query.Page=nil, got %v", *in.Query.Page)
		}
		if in.Header.TraceID != nil {
			t.Errorf("expected Header.TraceID=nil, got %v", *in.Header.TraceID)
		}
	})
}

func TestCompiler_SliceFields(t *testing.T) {
	type SliceRequest struct {
		Query struct {
			Tags []string `query:"tags"`
			IDs  []int    `query:"ids"`
		}
		Header struct {
			Values []int `header:"X-Values"`
		}
	}

	extractor, _ := bind.Compiler[SliceRequest]()

	req := httptest.NewRequest(http.MethodGet, "/?tags=go&tags=web&ids=1&ids=2", nil)
	req.Header.Add("X-Values", "10")
	req.Header.Add("X-Values", "20")

	var in SliceRequest
	v := reflect.ValueOf(&in).Elem()

	if err := extractor(context.Background(), req, v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Query Slices
	if len(in.Query.Tags) != 2 || in.Query.Tags[0] != "go" || in.Query.Tags[1] != "web" {
		t.Errorf("unexpected Query.Tags: %v", in.Query.Tags)
	}
	if len(in.Query.IDs) != 2 || in.Query.IDs[0] != 1 || in.Query.IDs[1] != 2 {
		t.Errorf("unexpected Query.IDs: %v", in.Query.IDs)
	}

	// Verify Header Slice
	if len(in.Header.Values) != 2 || in.Header.Values[0] != 10 || in.Header.Values[1] != 20 {
		t.Errorf("unexpected Header.Values: %v", in.Header.Values)
	}
}

func TestCompiler_Schema(t *testing.T) {
	type SchemaRequest struct {
		Path struct {
			ID int `path:"id"`
		}
		Query struct {
			Page *int `query:"page"`
		}
		Body struct {
			Name string `json:"name"`
		}
	}

	_, schema := bind.Compiler[SchemaRequest]()

	if len(schema.Parameters) != 2 {
		t.Errorf("expected 2 parameters, got %d", len(schema.Parameters))
	}

	// Verify Path Parameter
	if schema.Parameters[0].Name != "id" || schema.Parameters[0].In != "path" || !schema.Parameters[0].Required {
		t.Errorf("unexpected path parameter: %+v", schema.Parameters[0])
	}

	// Verify Query Parameter
	if schema.Parameters[1].Name != "page" || schema.Parameters[1].In != "query" || schema.Parameters[1].Required {
		t.Errorf("unexpected query parameter: %+v", schema.Parameters[1])
	}

	// Verify Body
	if schema.Body == nil || schema.Body.Name() != "" { // Body is an anonymous struct here
		if schema.Body == nil {
			t.Error("expected body schema to be present")
		}
	}
}

func TestCompiler_Validation(t *testing.T) {
	extractor, _ := bind.Compiler[ValidationRequest]()

	// Invalid input
	body := bytes.NewBufferString(`{"age": 16}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	
	var in ValidationRequest
	v := reflect.ValueOf(&in).Elem()

	err := extractor(context.Background(), req, v)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if err.Error() != "age must be at least 18" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	// Valid input
	body = bytes.NewBufferString(`{"age": 21}`)
	req = httptest.NewRequest(http.MethodPost, "/", body)
	in = ValidationRequest{}
	v = reflect.ValueOf(&in).Elem()

	err = extractor(context.Background(), req, v)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestCompiler_FullExtraction(t *testing.T) {
	type FullRequest struct {
		Path struct {
			ID int `path:"id"`
		}
		Query struct {
			Verbose bool `query:"verbose"`
		}
		Header struct {
			XRequestID string `header:"X-Request-Id"`
		}
		Body struct {
			Name string `json:"name"`
		}
	}

	extractor, _ := bind.Compiler[FullRequest]()

	// Mock request
	body := bytes.NewBufferString(`{"name": "aku"}`)
	req := httptest.NewRequest(http.MethodPost, "/users/123?verbose=true", body)
	req.Header.Set("X-Request-Id", "req-456")
	req.Header.Set("Content-Type", "application/json")
	
	// Go 1.22 path values mock
	req.SetPathValue("id", "123")

	var in FullRequest
	v := reflect.ValueOf(&in).Elem()

	err := extractor(context.Background(), req, v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Path
	if in.Path.ID != 123 {
		t.Errorf("expected Path.ID=123, got %d", in.Path.ID)
	}

	// Verify Query
	if !in.Query.Verbose {
		t.Errorf("expected Query.Verbose=true, got %v", in.Query.Verbose)
	}

	// Verify Header
	if in.Header.XRequestID != "req-456" {
		t.Errorf("expected Header.XRequestID=req-456, got %s", in.Header.XRequestID)
	}

	// Verify Body
	if in.Body.Name != "aku" {
		t.Errorf("expected Body.Name=aku, got %s", in.Body.Name)
	}
}

func TestCompiler_CoercionErrors(t *testing.T) {
	type PathRequest struct {
		Path struct {
			ID int `path:"id"`
		}
	}

	extractor, _ := bind.Compiler[PathRequest]()

	req := httptest.NewRequest(http.MethodGet, "/users/abc", nil)
	req.SetPathValue("id", "abc") // Not an integer

	var in PathRequest
	v := reflect.ValueOf(&in).Elem()

	err := extractor(context.Background(), req, v)
	if err == nil {
		t.Fatal("expected error for invalid integer coercion, got nil")
	}
}
func TestCompiler_MapFields(t *testing.T) {
	type MapRequest struct {
		Query struct {
			Filters map[string]string `query:"filter"`
			Scores  map[string]int    `query:"score"`
		}
		Header struct {
			Metadata map[string]string `header:"X-Meta-"`
			Tags     map[string]bool   `header:"tags"`
		}
	}

	extractor, _ := bind.Compiler[MapRequest]()

	req := httptest.NewRequest(http.MethodGet, "/?filter[name]=nick&filter[type]=admin&score[rank]=1&score[lvl]=99", nil)
	req.Header.Set("X-Meta-Source", "github")
	req.Header.Set("X-Meta-Env", "prod")
	req.Header.Set("tags[verified]", "true")
	req.Header.Set("tags[legacy]", "false")

	var in MapRequest
	v := reflect.ValueOf(&in).Elem()

	if err := extractor(context.Background(), req, v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Query Maps
	if in.Query.Filters["name"] != "nick" || in.Query.Filters["type"] != "admin" {
		t.Errorf("unexpected Query.Filters: %v", in.Query.Filters)
	}
	if in.Query.Scores["rank"] != 1 || in.Query.Scores["lvl"] != 99 {
		t.Errorf("unexpected Query.Scores: %v", in.Query.Scores)
	}

	// Verify Header Maps
	if in.Header.Metadata["Source"] != "github" || in.Header.Metadata["Env"] != "prod" {
		t.Errorf("unexpected Header.Metadata: %v", in.Header.Metadata)
	}
	if !in.Header.Tags["verified"] || in.Header.Tags["legacy"] {
		t.Errorf("unexpected Header.Tags: %v", in.Header.Tags)
	}
}
