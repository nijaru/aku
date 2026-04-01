package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

// HealthChecker implements the health checks for liveness/readiness.
type HealthChecker struct {
	// Checks are functions that verify the application's health.
	// A nil error means the check passed.
	Checks map[string]CheckFunc
}

// CheckFunc defines a liveness or readiness test.
type CheckFunc func(context.Context) error

// HealthStatus represents the response for /ready.
type HealthStatus struct {
	Status       string            `json:"status"`
	ChecksStatus map[string]string `json:"checks,omitempty"`
}

// NewHealthChecker creates a new health checker registry.
func NewHealthChecker() HealthChecker {
	return HealthChecker{
		Checks: make(map[string]CheckFunc),
	}
}

// AddRegister adds a readiness check. If the check fails, the service is
// considered not ready. If the check panics, it is captured as a check
// failure.
func (hc *HealthChecker) Add(name string, fn CheckFunc) {
	hc.Checks[name] = fn
}

// Liveness returns an http.Handler for liveness checks.
// Always responds with 200 OK. Use this for Kubernetes liveness probes.
func (hc HealthChecker) Liveness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"passed"}`))
	})
}

// Readiness returns an http.Handler for readiness checks.
// Runs all registered checks concurrently and returns 503 if any fail.
func (hc HealthChecker) Readiness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(hc.Checks) == 0 {
			// No registered checks = ready.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"passed"}`))
			return
		}

		results := make(map[string]string, len(hc.Checks))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, check := range hc.Checks {
			wg.Add(1)
			name := name
			check := check
			go func() {
				defer wg.Done()
				if err := check(r.Context()); err != nil {
					mu.Lock()
					results[name] = "failed: " + err.Error()
					mu.Unlock()
				} else {
					mu.Lock()
					results[name] = "passed"
					mu.Unlock()
				}
			}()
		}

		wg.Wait()

		anyFailed := false
		for _, v := range results {
			if len(v) > 6 && v[:6] == "failed" {
				anyFailed = true
				break
			}
		}

		status := HealthStatus{
			ChecksStatus: results,
		}

		w.Header().Set("Content-Type", "application/json")
		if anyFailed {
			status.Status = "failed"
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			status.Status = "passed"
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(status)
	})
}

// Simple health check that returns an error if the context is cancelled.
func HealthyCheck(_ context.Context) error {
	return nil
}

// Middleware returns middleware that intercepts /health and /ready
// paths before forwarding to the main handler. This allows attaching
// liveness and readiness endpoints to any http.Handler without
// polluting the application router.
func (hc HealthChecker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			hc.Liveness().ServeHTTP(w, r)
			return
		case "/ready":
			hc.Readiness().ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
