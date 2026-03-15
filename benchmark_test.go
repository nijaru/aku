package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

type BenchInput struct {
	Path struct {
		ID string `path:"id"`
	}
	Query struct {
		Filter string `query:"filter"`
		Page   int    `query:"page"`
	}
}

type BenchOutput struct {
	ID     string `json:"id"`
	Filter string `json:"filter"`
	Page   int    `json:"page"`
}

func benchHandler(ctx context.Context, in BenchInput) (BenchOutput, error) {
	return BenchOutput{
		ID:     in.Path.ID,
		Filter: in.Query.Filter,
		Page:   in.Query.Page,
	}, nil
}

func BenchmarkHandler(b *testing.B) {
	app := aku.New()
	aku.Get(app, "/test/{id}", benchHandler)

	req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
	w := httptest.NewRecorder()

	for b.Loop() {
		app.ServeHTTP(w, req)
	}
}

func BenchmarkStdlib(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Manual binding similar to what aku does
		id := r.PathValue("id")
		filter := r.URL.Query().Get("filter")
		page := r.URL.Query().Get("page")
		_, _ = id, filter
		_, _ = page, id
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"123","filter":"active","page":1}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
	w := httptest.NewRecorder()

	for b.Loop() {
		mux.ServeHTTP(w, req)
	}
}
