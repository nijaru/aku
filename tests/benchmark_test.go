package aku_test

import (
	"context"
	"encoding/json"
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

	b.ResetTimer()
	for b.Loop() {
		app.ServeHTTP(w, req)
	}
}

func BenchmarkStdlib(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		query := r.URL.Query()
		filter := query.Get("filter")
		page := 1

		out := BenchOutput{
			ID:     id,
			Filter: filter,
			Page:   page,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()
	for b.Loop() {
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkStdlibMarshal(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		query := r.URL.Query()
		filter := query.Get("filter")
		page := 1

		out := BenchOutput{
			ID:     id,
			Filter: filter,
			Page:   page,
		}

		data, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})

	req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()
	for b.Loop() {
		mux.ServeHTTP(w, req)
	}
}
