package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthChecker_Liveness(t *testing.T) {
	hc := NewHealthChecker()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	hc.Liveness().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "passed" {
		t.Fatalf("expected status=passed, got %s", resp["status"])
	}
}

func TestHealthChecker_Readiness_NoChecks(t *testing.T) {
	hc := NewHealthChecker()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	hc.Readiness().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthChecker_Readiness_AllPassed(t *testing.T) {
	hc := NewHealthChecker()
	hc.Add("database", func(context.Context) error {
		return nil
	})
	hc.Add("redis", func(context.Context) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	hc.Readiness().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != "passed" {
		t.Fatalf("expected status=passed, got %s", status.Status)
	}
	if status.ChecksStatus["database"] != "passed" {
		t.Fatalf("expected database=passed")
	}
	if status.ChecksStatus["redis"] != "passed" {
		t.Fatalf("expected redis=passed")
	}
}

func TestHealthChecker_Readiness_OneFails(t *testing.T) {
	hc := NewHealthChecker()
	hc.Add("database", func(context.Context) error {
		return nil // passes
	})
	hc.Add("redis", func(context.Context) error {
		return errors.New("connection refused")
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	hc.Readiness().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Status != "failed" {
		t.Fatalf("expected status=failed, got %s", status.Status)
	}
	if status.ChecksStatus["database"] != "passed" {
		t.Fatalf("expected database=passed")
	}
	if status.ChecksStatus["redis"] != "failed: connection refused" {
		t.Fatalf("expected redis to fail with message, got: %s", status.ChecksStatus["redis"])
	}
}

func TestHealthChecker_Middleware(t *testing.T) {
	hc := NewHealthChecker()
	appHit := false

	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appHit = true
		w.WriteHeader(http.StatusOK)
	})
	handler := hc.Middleware(app)

	// /health should be intercepted
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health, got %d", rec.Code)
	}
	if appHit {
		t.Fatal("app handler should not have been called for /health")
	}

	// /ready should be intercepted
	appHit = false
	req = httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /ready, got %d", rec.Code)
	}
	if appHit {
		t.Fatal("app handler should not have been called for /ready")
	}

	// /users should forward to app
	appHit = false
	req = httptest.NewRequest(http.MethodGet, "/users", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !appHit {
		t.Fatal("expected app handler to be called for /users")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthyCheck(t *testing.T) {
	if err := HealthyCheck(context.Background()); err != nil {
		t.Fatalf("HealthyCheck should always return nil")
	}
}
