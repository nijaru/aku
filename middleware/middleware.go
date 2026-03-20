package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nijaru/aku/problem"
	"golang.org/x/time/rate"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
)

// RequestID returns a middleware that injects a unique ID into each request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request ID from the context, if any.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logger returns a middleware that logs HTTP requests using slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(lw, r)

		level := slog.LevelInfo
		if lw.status >= 500 {
			level = slog.LevelError
		} else if lw.status >= 400 {
			level = slog.LevelWarn
		}
		args := []any{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", lw.status),
			slog.Duration("duration", time.Since(start)),
			slog.Int("size", lw.size),
			slog.String("ip", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}

		if id := GetRequestID(r.Context()); id != "" {
			args = append(args, slog.String("request_id", id))
		}

		slog.Log(r.Context(), level, "http request", args...)
	})
}

// Recover returns a middleware that recovers from panics and logs them.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				args := []any{slog.Any("error", err)}
				if id := GetRequestID(r.Context()); id != "" {
					args = append(args, slog.String("request_id", id))
				}
				slog.Error("panic recovered", args...)
				// We don't use handleError here because we want to keep middleware independent
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"type":"https://aku.sh/problems/internal-error","title":"Internal Server Error","status":500}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Timeout returns a middleware that cancels the request context after a duration.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Limit returns a middleware that rate limits requests using a token bucket.
// It is a simple global limiter for the route(s) it is applied to.
func Limit(rps float64, burst int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				prob := problem.TooManyRequests("Rate limit exceeded")
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(prob.Status)
				// Small hack to marshal JSON since we don't have access to render here easily
				// Alternatively, we could just copy the JSON render logic for this single case
				w.Write([]byte(`{"type":"` + prob.Type + `","title":"` + prob.Title + `","status":` + strconv.Itoa(prob.Status) + `,"detail":"` + prob.Detail + `"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSOptions configures the CORS middleware.
type CORSOptions struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposedHeaders []string
	MaxAge         int
}

// CORS returns a middleware that implements Cross-Origin Resource Sharing.
func CORS(opts CORSOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := false
			for _, o := range opts.AllowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if !allowed {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			if r.Method == http.MethodOptions {
				if len(opts.AllowedMethods) > 0 {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(opts.AllowedMethods, ", "))
				}
				if len(opts.AllowedHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(opts.AllowedHeaders, ", "))
				}
				if opts.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(opts.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if len(opts.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(opts.ExposedHeaders, ", "))
			}

			next.ServeHTTP(w, r)
		})
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
