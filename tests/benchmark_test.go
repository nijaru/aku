package aku_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// BenchmarkAku measures the full Aku pipeline: routing, extraction, validation, and rendering.
func BenchmarkAku(b *testing.B) {
	app := aku.New()
	aku.Get(app, "/test/{id}", benchHandler)

	req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

// BenchmarkAkuParallel measures the Aku pipeline under concurrent load.
func BenchmarkAkuParallel(b *testing.B) {
	app := aku.New()
	aku.Get(app, "/test/{id}", benchHandler)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
		w := newBenchmarkResponseWriter()
		for pb.Next() {
			w.reset()
			app.ServeHTTP(w, req)
		}
	})
}

// BenchmarkAkuHandleHTTP measures the "escape hatch" for standard handlers.
func BenchmarkAkuHandleHTTP(b *testing.B) {
	app := aku.New()
	app.HandleHTTP(
		http.MethodGet,
		"/test/{id}",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}),
		aku.WithSummary("benchmark handler"),
	)

	req := httptest.NewRequest(http.MethodGet, "/test/123", nil)
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w.reset()
		app.ServeHTTP(w, req)
	}
}

// BenchmarkStdlib measures the Go 1.22+ ServeMux with equivalent manual logic.
func BenchmarkStdlib(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		query := r.URL.Query()
		filter := query.Get("filter")
		pageStr := query.Get("page")
		page, _ := strconv.Atoi(pageStr)

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
	w := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w.reset()
		mux.ServeHTTP(w, req)
	}
}

// BenchmarkStdlibParallel measures the standard library under concurrent load.
func BenchmarkStdlibParallel(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		query := r.URL.Query()
		filter := query.Get("filter")
		pageStr := query.Get("page")
		page, _ := strconv.Atoi(pageStr)

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

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "/test/123?filter=active&page=1", nil)
		w := newBenchmarkResponseWriter()
		for pb.Next() {
			w.reset()
			mux.ServeHTTP(w, req)
		}
	})
}
