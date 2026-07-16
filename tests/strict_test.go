package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/auth"
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

func TestStrictQueryAppliesWithoutQuerySection(t *testing.T) {
	app := aku.New(aku.WithStrictQuery())
	aku.Get(app, "/test", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test?unexpected=value", nil)
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected strict query rejection, got %d: %s", rr.Code, rr.Body.String())
	}
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

	t.Run("MissingRequiredHeader", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
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

func TestStrictHeaderAllowsTypedBearerAuth(t *testing.T) {
	type In struct {
		Header struct {
			Trace string `header:"X-Trace"`
		}
		Auth struct {
			Token auth.Bearer
		}
	}

	app := aku.New(aku.WithStrictHeader())
	aku.Get(app, "/test", func(ctx context.Context, in In) (string, error) {
		return string(in.Auth.Token), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace", "trace")
	req.Header.Set("User-Agent", "test")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf(
			"expected auth header to be accepted in strict mode, got %d: %s",
			rr.Code,
			rr.Body.String(),
		)
	}
}

func TestStrictHeaderAllowsMiddlewareAuthAndCookies(t *testing.T) {
	app := aku.New(aku.WithStrictHeader())
	app.Use(auth.RequireBearer())
	if err := aku.Get(app, "/test", func(ctx context.Context, in struct{}) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Cookie", "session=present")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected middleware auth and cookie headers to pass strict mode, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}

func TestStrictQueryAllowsAPIKeyAuth(t *testing.T) {
	type In struct {
		Auth struct {
			Key auth.APIKey `auth:"apikey:query:api_key"`
		}
	}

	app := aku.New(aku.WithStrictQuery())
	aku.Get(app, "/test", func(ctx context.Context, in In) (string, error) {
		return string(in.Auth.Key), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test?api_key=secret", nil)
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf(
			"expected auth query parameter to be accepted in strict mode, got %d: %s",
			rr.Code,
			rr.Body.String(),
		)
	}
}
