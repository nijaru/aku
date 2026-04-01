package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState_Allows(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.State() != StateClosed {
		t.Fatalf("expected closed, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("expected Allow() in closed state")
	}
}

func TestCircuitBreaker_ThresholdOpening(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
	})

	// Trip the breaker with 3 failures
	for i := range 3 {
		if !cb.Allow() {
			t.Fatalf("expected Allow on failure %d", i+1)
		}
		cb.Record(false)
	}

	// Should now be open
	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	// Should reject
	if cb.Allow() {
		t.Fatal("expected Reject in open state")
	}
}

func TestCircuitBreaker_RecoveryToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
	})

	// Open the circuit
	cb.Allow()
	cb.Record(false)
	cb.Allow()
	cb.Record(false)

	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	state := cb.State()
	if state != StateHalfOpen {
		t.Fatalf("expected half-open after timeout, got %v", state)
	}
}

func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
		SuccessThreshold: 1,
	})

	// Open
	cb.Allow()
	cb.Record(false)
	cb.Allow()
	cb.Record(false)

	time.Sleep(60 * time.Millisecond)
	cb.State() // triggers transition to half-open

	// Allow probe through
	if !cb.Allow() {
		t.Fatal("expected Allow in half-open")
	}

	// Success should close
	cb.Record(true)
	if cb.State() != StateClosed {
		t.Fatalf("expected closed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  30 * time.Millisecond,
	})

	// Open
	cb.Allow()
	cb.Record(false)
	cb.Allow()
	cb.Record(false)

	time.Sleep(40 * time.Millisecond)
	cb.State()

	// Allow probe, then fail
	if !cb.Allow() {
		t.Fatal("expected Allow in half-open")
	}
	cb.Record(false)

	if cb.State() != StateOpen {
		t.Fatalf("expected open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1})

	cb.Allow()
	cb.Record(false)

	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %v", cb.State())
	}

	cb.Reset()
	if cb.State() != StateClosed {
		t.Fatalf("expected closed after reset, got %v", cb.State())
	}
}

func TestCircuitBreakerMiddleware_Allow(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mw := CircuitBreakerMiddleware(cb)(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCircuitBreakerMiddleware_OpenRejects(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
	})
	// Manually open
	cb.Allow()
	cb.Record(false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})
	mw := CircuitBreakerMiddleware(cb)(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCircuitBreakerMiddleware_5xxCounts(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mw := CircuitBreakerMiddleware(cb)(handler)

	// Two 500s should trip the breaker
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
	}

	// Third request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after circuit open, got %d", rec.Code)
	}
}

func TestCircuitBreakerMiddleware_CustomFailureFn(t *testing.T) {
	// Count 429 as failures too
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		IsFailure: func(status int) bool {
			return status >= 500 || status == http.StatusTooManyRequests
		},
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	mw := CircuitBreakerMiddleware(cb)(handler)

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestCircuitBreakerGroup(t *testing.T) {
	g := NewCircuitBreakerGroup(CircuitBreakerConfig{FailureThreshold: 2})

	cb1 := g.Get("users")
	cb2 := g.Get("orders")

	// Open one, the other should be independent
	cb1.Allow()
	cb1.Record(false)
	cb1.Allow()
	cb1.Record(false)

	if cb1.State() != StateOpen {
		t.Fatal("users should be open")
	}
	if cb2.State() != StateClosed {
		t.Fatal("orders should still be closed")
	}

	all := g.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 breakers, got %d", len(all))
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var changes atomic.Int32
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		OnStateChange: func(prev, next State, msg string) {
			changes.Add(1)
		},
	})

	cb.Allow()
	cb.Record(false)

	// Give the goroutine a moment
	time.Sleep(10 * time.Millisecond)

	c := changes.Load()
	if c == 0 {
		t.Fatal("expected OnStateChange callback to fire")
	}
}
