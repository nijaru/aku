package bind_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nijaru/aku/internal/bind"
)

func TestCompiler_EmptyStruct(t *testing.T) {
	type Empty struct{}

	// Compilation should succeed without panicking
	extractor := bind.Compiler[Empty]()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var in Empty
	v := reflect.ValueOf(&in).Elem()

	// Extraction should succeed with no error
	err := extractor(context.Background(), req, v)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCompiler_NotStruct(t *testing.T) {
	// Even though the framework shouldn't normally allow this (via type constraints later),
	// the compiler should handle non-structs safely.
	extractor := bind.Compiler[int]()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var in int
	v := reflect.ValueOf(&in).Elem()

	err := extractor(context.Background(), req, v)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
