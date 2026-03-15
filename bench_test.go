package aku_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

type BenchRequest struct {
	Path struct {
		ID string `path:"id"`
	}
	Query struct {
		Verbose bool `query:"verbose"`
	}
	Body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}
}

func (r BenchRequest) Validate() error {
	if r.Body.Age < 0 {
		return errors.New("age must be positive")
	}
	return nil
}

type BenchResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Verbose bool   `json:"verbose"`
}

func BenchmarkAku(b *testing.B) {
	app := aku.New()
	aku.Post(app, "/users/{id}", func(ctx context.Context, in BenchRequest) (BenchResponse, error) {
		return BenchResponse{
			ID:      in.Path.ID,
			Name:    in.Body.Name,
			Verbose: in.Query.Verbose,
		}, nil
	})

	body, _ := json.Marshal(map[string]any{
		"name":  "Nick",
		"email": "nick@example.com",
		"age":   30,
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/users/123?verbose=true", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.ServeHTTP(w, req)
	}
}

func BenchmarkNetHTTP(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		verbose := r.URL.Query().Get("verbose") == "true"

		var body struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Age   int    `json:"age"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Manual Validation
		if body.Age < 0 {
			http.Error(w, "age must be positive", http.StatusUnprocessableEntity)
			return
		}

		resp := BenchResponse{
			ID:      id,
			Name:    body.Name,
			Verbose: verbose,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	bodyBytes, _ := json.Marshal(map[string]any{
		"name":  "Nick",
		"email": "nick@example.com",
		"age":   30,
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/users/123?verbose=true", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(w, req)
	}
}
