package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

func TestStrictQuery(t *testing.T) {
	type In struct {
		Query struct {
			Name string `query:"name"`
		}
	}

	app := aku.New(aku.WithStrictQuery())
	aku.Get(app, "/test", func(ctx context.Context, in In) (any, error) {
		return map[string]string{"name": in.Query.Name}, nil
	})

	t.Run("Valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?name=foo", nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("StrictViolation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?name=foo&extra=bar", nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}

func TestStrictHeader(t *testing.T) {
	type In struct {
		Header struct {
			ApiKey string `header:"X-Api-Key"`
		}
	}

	app := aku.New(aku.WithStrictHeader())
	aku.Get(app, "/test", func(ctx context.Context, in In) (any, error) {
		return map[string]string{"key": in.Header.ApiKey}, nil
	})

	t.Run("Valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Api-Key", "secret")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("StrictViolation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Api-Key", "secret")
		req.Header.Set("X-Extra", "foo")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("StandardHeadersIgnored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Api-Key", "secret")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}
