package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

// HealthChecker manages liveness and readiness checks.
// Safe for concurrent use.
type HealthChecker struct {
	Checks map[string]CheckFunc
}

// CheckFunc defines a liveness or readiness verification.
// A nil return value indicates the check passed.
type CheckFunc func(context.Context) error

// HealthStatus is the JSON response shape for /ready.
type HealthStatus struct {
	Status       string            `json:"status"`
	ChecksStatus map[string]string `json:"checks,omitempty"`
}

const passedResponse = `{"status":"passed"}`

// NewHealthChecker creates an empty HealthChecker.
func NewHealthChecker() HealthChecker {
	return HealthChecker{Checks: make(map[string]CheckFunc)}
}

// Add registers a named readiness check.
// Panics on nil check function.
func (hc *HealthChecker) Add(name string, fn CheckFunc) {
	hc.Checks[name] = fn
}

// Liveness returns an http.Handler that always responds 200 OK.
// Use for Kubernetes liveness probes.
func (hc *HealthChecker) Liveness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(passedResponse))
	})
}

// Readiness returns an http.Handler that runs all registered checks
// concurrently. Responses 200 when all pass, 503 when any fail.
func (hc *HealthChecker) Readiness() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(hc.Checks) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(passedResponse))
			return
		}

		results := make(map[string]string, len(hc.Checks))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, check := range hc.Checks {
			wg.Go(func() {
				if err := check(r.Context()); err != nil {
					mu.Lock()
					results[name] = "failed: " + err.Error()
					mu.Unlock()
				} else {
					mu.Lock()
					results[name] = "passed"
					mu.Unlock()
				}
			})
		}

		wg.Wait()

		var anyFailed bool
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

// HealthyCheck is a no-op check that always passes.
func HealthyCheck(_ context.Context) error {
	return nil
}

// Middleware intercepts /health and /ready before forwarding to next.
// This allows attaching health endpoints to any http.Handler without
// polluting the application router.
func (hc *HealthChecker) Middleware(next http.Handler) http.Handler {
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
