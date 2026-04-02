package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// State represents the state of a circuit breaker.
type State int

const (
	StateClosed   State = iota // Normal operation — requests pass through.
	StateOpen                  // Circuit tripped — requests are rejected immediately.
	StateHalfOpen              // Testing recovery — one probe request is allowed.
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	// Default: 5.
	FailureThreshold int

	// RecoveryTimeout is how long the circuit stays open before entering half-open.
	// Default: 30 seconds.
	RecoveryTimeout time.Duration

	// SuccessThreshold is the number of consecutive successes in half-open state
	// required to close the circuit. Default: 1.
	SuccessThreshold int

	// OnStateChange is called when the circuit transitions between states.
	// Receives the previous state, new state, and the reason message.
	OnStateChange func(prev, next State, msg string)

	// IsFailure determines whether a response code counts as a failure.
	// Default: status >= 500.
	IsFailure func(status int) bool
}

func (c *CircuitBreakerConfig) applyDefaults() {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.RecoveryTimeout <= 0 {
		c.RecoveryTimeout = 30 * time.Second
	}
	if c.SuccessThreshold <= 0 {
		c.SuccessThreshold = 1
	}
	if c.IsFailure == nil {
		c.IsFailure = func(status int) bool { return status >= 500 }
	}
}

// CircuitBreaker implements the circuit breaker pattern for HTTP handlers.
// Safe for concurrent use.
type CircuitBreaker struct {
	cfg CircuitBreakerConfig

	mu            sync.Mutex
	state         State
	failures      int
	success       int
	openedAt      time.Time
	probeInFlight bool
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	cfg.applyDefaults()
	return &CircuitBreaker{cfg: cfg, state: StateClosed}
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkTransition()
	return cb.state
}

// Allow checks whether a request should be permitted through.
// If true is returned, the caller MUST call cb.Record(success) after
// the result is known.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkTransition()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		if cb.probeInFlight {
			return false
		}
		// Allow exactly one probe through.
		cb.probeInFlight = true
		return true
	default:
		return false
	}
}

// Record records the outcome of a request. success=true for success, false for failure.
func (cb *CircuitBreaker) Record(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	defer func() {
		cb.probeInFlight = false
	}()

	if success {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

// Reset forces the circuit breaker back to the closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transition(StateClosed, "manual reset")
}

func (cb *CircuitBreaker) checkTransition() {
	if cb.state == StateOpen {
		if time.Since(cb.openedAt) >= cb.cfg.RecoveryTimeout {
			cb.transition(StateHalfOpen, "recovery timeout elapsed")
		}
	}
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.success++
		if cb.success >= cb.cfg.SuccessThreshold {
			cb.transition(StateClosed, "success threshold reached")
		}
	case StateOpen:
		cb.failures = 0
		cb.transition(StateClosed, "success after open")
	}
}

func (cb *CircuitBreaker) onFailure() {
	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.transition(StateOpen, "failure threshold reached")
		}
	case StateHalfOpen:
		cb.transition(StateOpen, "probe request failed")
		cb.success = 0
	case StateOpen:
		// Already open — reset the recovery timer.
		cb.openedAt = time.Now()
	}
}

func (cb *CircuitBreaker) transition(next State, msg string) {
	prev := cb.state
	cb.state = next
	cb.openedAt = time.Now()
	cb.probeInFlight = false

	if next == StateClosed {
		cb.failures = 0
		cb.success = 0
	}

	if cb.cfg.OnStateChange != nil {
		prevCopy := prev
		nextCopy := next
		msgCopy := msg
		go cb.cfg.OnStateChange(prevCopy, nextCopy, msgCopy)
	}
}

// CircuitBreakerMiddleware returns middleware that protects the downstream
// handler with a circuit breaker. When the circuit is open, requests
// receive a 503 Service Unavailable with a Problem Details response.
//
// Failures are detected by the response status code (default: ≥ 500).
func CircuitBreakerMiddleware(cb *CircuitBreaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cb.Allow() {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusServiceUnavailable)
				resp := map[string]any{
					"type":   "about:blank",
					"title":  "Service Unavailable",
					"status": http.StatusServiceUnavailable,
					"detail": "Circuit breaker is open — request rejected",
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			rec := &statusCapture{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rec, r)
			cb.Record(!cb.cfg.IsFailure(rec.code))
		})
	}
}

// CircuitBreakerGroup manages independent circuit breakers for multiple
// downstream services or endpoints.
type CircuitBreakerGroup struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	cfg      CircuitBreakerConfig
}

// NewCircuitBreakerGroup creates a registry of circuit breakers
// shareable across routes or groups.
func NewCircuitBreakerGroup(cfg CircuitBreakerConfig) *CircuitBreakerGroup {
	cfg.applyDefaults()
	return &CircuitBreakerGroup{breakers: make(map[string]*CircuitBreaker), cfg: cfg}
}

// Get returns the circuit breaker for a named endpoint, creating one
// lazily if it doesn't exist.
func (g *CircuitBreakerGroup) Get(name string) *CircuitBreaker {
	g.mu.RLock()
	cb, ok := g.breakers[name]
	g.mu.RUnlock()
	if ok {
		return cb
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if cb, ok = g.breakers[name]; ok {
		return cb
	}
	cb = NewCircuitBreaker(g.cfg)
	g.breakers[name] = cb
	return cb
}

// All returns the current state of every registered breaker.
func (g *CircuitBreakerGroup) All() map[string]State {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make(map[string]State, len(g.breakers))
	for name, cb := range g.breakers {
		result[name] = cb.State()
	}
	return result
}

type statusCapture struct {
	http.ResponseWriter
	code int
}

func (w *statusCapture) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
