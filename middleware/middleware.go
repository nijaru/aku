package middleware

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/google/uuid"
	"github.com/klauspost/compress/gzhttp"
	"github.com/nijaru/aku/internal/render"
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
				render.Problem(w, http.StatusInternalServerError,
					problem.InternalServerError("panic recovered"))
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
				render.Problem(w, prob.Status, prob)
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
			addVary(w.Header(), "Origin")

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
				addVary(w.Header(), "Access-Control-Request-Method")
				addVary(w.Header(), "Access-Control-Request-Headers")
				if len(opts.AllowedMethods) > 0 {
					w.Header().
						Set("Access-Control-Allow-Methods", strings.Join(opts.AllowedMethods, ", "))
				}
				if len(opts.AllowedHeaders) > 0 {
					w.Header().
						Set("Access-Control-Allow-Headers", strings.Join(opts.AllowedHeaders, ", "))
				}
				if opts.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(opts.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if len(opts.ExposedHeaders) > 0 {
				w.Header().
					Set("Access-Control-Expose-Headers", strings.Join(opts.ExposedHeaders, ", "))
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

func (w *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (w *loggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

var brotliPool = sync.Pool{
	New: func() any {
		return brotli.NewWriter(nil)
	},
}

// Compress returns a middleware that compresses HTTP responses using Brotli, Zstandard, or Gzip.
// It prioritizes Brotli, then Zstandard, then Gzip.
func Compress(next http.Handler) http.Handler {
	// Use klauspost's gzhttp for Gzip and Zstd (it handles pooling, etags, etc.)
	gzstd := gzhttp.GzipHandler(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ae := r.Header.Get("Accept-Encoding")

		// 1. Brotli (Custom pooled implementation)
		if acceptsEncoding(ae, "br") {
			// Check if we should compress based on content type if possible,
			// but we often don't know it until WriteHeader is called.
			bw := brotliPool.Get().(*brotli.Writer)
			bw.Reset(w)

			cw := &brotliResponseWriter{
				ResponseWriter: w,
				writer:         bw,
			}
			defer func() {
				if cw.wrote && cw.Header().Get("Content-Encoding") == "br" {
					bw.Close()
				}
				brotliPool.Put(bw)
			}()

			next.ServeHTTP(cw, r)
			return
		}

		// 2. Zstandard / Gzip (via gzhttp)
		gzstd.ServeHTTP(w, r)
	})
}

type brotliResponseWriter struct {
	http.ResponseWriter
	writer *brotli.Writer
	wrote  bool
}

func (w *brotliResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true

	ct := w.Header().Get("Content-Type")
	if isCompressible(ct) {
		w.Header().Set("Content-Encoding", "br")
		addVary(w.Header(), "Accept-Encoding")
		w.ResponseWriter.WriteHeader(status)
	} else {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *brotliResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		// Try to detect content type if not set
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}

	if w.Header().Get("Content-Encoding") == "br" {
		return w.writer.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *brotliResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *brotliResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (w *brotliResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// SecurityHeadersOptions configures the SecurityHeaders middleware.
// Zero values keep the secure defaults. Use DisabledHeaders to omit defaults.
type SecurityHeadersOptions struct {
	// ContentSecurityPolicy sets the Content-Security-Policy header.
	// Default: "default-src 'self'; object-src 'none'; base-uri 'none'"
	ContentSecurityPolicy string

	// HSTSMaxAge sets the max-age for Strict-Transport-Security in seconds.
	// Set to negative to disable HSTS. Default: 63072000 (2 years, per OWASP).
	HSTSMaxAge int

	// HSTSIncludeSubDomains adds includeSubDomains to the HSTS directive.
	HSTSIncludeSubDomains bool

	// HSTSPreload adds preload to the HSTS directive.
	// WARNING: Once submitted to the HSTS preload list, removal takes months.
	HSTSPreload bool

	// XFrameOptions sets X-Frame-Options. Default: "DENY".
	// Valid values: "DENY", "SAMEORIGIN", "" (disabled).
	XFrameOptions string

	// ReferrerPolicy sets the Referrer-Policy header.
	// Default: "strict-origin-when-cross-origin".
	ReferrerPolicy string

	// PermissionsPolicy sets the Permissions-Policy header.
	// Default disables camera, microphone, geolocation, payment, USB, and FLoC.
	PermissionsPolicy string

	// CrossOriginEmbedderPolicy sets Cross-Origin-Embedder-Policy.
	// Default: "" (not set). Set to "require-corp" only if you need SharedArrayBuffer.
	CrossOriginEmbedderPolicy string

	// CrossOriginOpenerPolicy sets Cross-Origin-Opener-Policy.
	// Default: "same-origin".
	CrossOriginOpenerPolicy string

	// CrossOriginResourcePolicy sets Cross-Origin-Resource-Policy.
	// Default: "same-origin".
	CrossOriginResourcePolicy string

	// XXSSProtection sets the X-XSS-Protection header.
	// OWASP recommends "0" (disable the legacy XSS auditor, which itself had vulns).
	// Default: "0".
	XXSSProtection string

	// DisabledHeaders omits specific default headers by canonical header name.
	// Example: []string{"Content-Security-Policy", "X-Frame-Options"}.
	DisabledHeaders []string
}

// SecurityHeaders returns a middleware that sets common security headers.
// It applies sensible defaults that can be overridden via SecurityHeadersOptions.
func SecurityHeaders(opts ...SecurityHeadersOptions) func(http.Handler) http.Handler {
	cfg := SecurityHeadersOptions{
		ContentSecurityPolicy:     "default-src 'self'; object-src 'none'; base-uri 'none'",
		HSTSMaxAge:                63072000,
		XFrameOptions:             "DENY",
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		PermissionsPolicy:         "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=(), interest-cohort=()",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
		XXSSProtection:            "0",
	}

	disabled := map[string]bool{}
	if len(opts) > 0 {
		o := opts[0]
		disabled = disabledHeaders(o.DisabledHeaders)
		if o.ContentSecurityPolicy != "" {
			cfg.ContentSecurityPolicy = o.ContentSecurityPolicy
		}
		if o.HSTSMaxAge != 0 {
			cfg.HSTSMaxAge = o.HSTSMaxAge
		}
		if o.XFrameOptions != "" {
			cfg.XFrameOptions = o.XFrameOptions
		}
		if o.ReferrerPolicy != "" {
			cfg.ReferrerPolicy = o.ReferrerPolicy
		}
		if o.PermissionsPolicy != "" {
			cfg.PermissionsPolicy = o.PermissionsPolicy
		}
		if o.CrossOriginEmbedderPolicy != "" {
			cfg.CrossOriginEmbedderPolicy = o.CrossOriginEmbedderPolicy
		}
		if o.CrossOriginOpenerPolicy != "" {
			cfg.CrossOriginOpenerPolicy = o.CrossOriginOpenerPolicy
		}
		if o.CrossOriginResourcePolicy != "" {
			cfg.CrossOriginResourcePolicy = o.CrossOriginResourcePolicy
		}
		if o.XXSSProtection != "" {
			cfg.XXSSProtection = o.XXSSProtection
		}
		cfg.HSTSIncludeSubDomains = o.HSTSIncludeSubDomains
		cfg.HSTSPreload = o.HSTSPreload
		if disabled["Strict-Transport-Security"] {
			cfg.HSTSMaxAge = -1
		}
		if disabled["Content-Security-Policy"] {
			cfg.ContentSecurityPolicy = ""
		}
		if disabled["X-Frame-Options"] {
			cfg.XFrameOptions = ""
		}
		if disabled["Referrer-Policy"] {
			cfg.ReferrerPolicy = ""
		}
		if disabled["Permissions-Policy"] {
			cfg.PermissionsPolicy = ""
		}
		if disabled["Cross-Origin-Embedder-Policy"] {
			cfg.CrossOriginEmbedderPolicy = ""
		}
		if disabled["Cross-Origin-Opener-Policy"] {
			cfg.CrossOriginOpenerPolicy = ""
		}
		if disabled["Cross-Origin-Resource-Policy"] {
			cfg.CrossOriginResourcePolicy = ""
		}
		if disabled["X-XSS-Protection"] {
			cfg.XXSSProtection = ""
		}
	}

	// Pre-compute the HSTS value since it doesn't change per-request.
	hsts := ""
	if cfg.HSTSMaxAge > 0 {
		hsts = "max-age=" + strconv.Itoa(cfg.HSTSMaxAge)
		if cfg.HSTSIncludeSubDomains {
			hsts += "; includeSubDomains"
		}
		if cfg.HSTSPreload {
			hsts += "; preload"
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			if cfg.ContentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
			}
			if hsts != "" {
				h.Set("Strict-Transport-Security", hsts)
			}
			if cfg.XFrameOptions != "" {
				h.Set("X-Frame-Options", cfg.XFrameOptions)
			}
			if !disabled["X-Content-Type-Options"] {
				h.Set("X-Content-Type-Options", "nosniff")
			}
			if cfg.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", cfg.ReferrerPolicy)
			}
			if cfg.PermissionsPolicy != "" {
				h.Set("Permissions-Policy", cfg.PermissionsPolicy)
			}
			if cfg.CrossOriginEmbedderPolicy != "" {
				h.Set("Cross-Origin-Embedder-Policy", cfg.CrossOriginEmbedderPolicy)
			}
			if cfg.CrossOriginOpenerPolicy != "" {
				h.Set("Cross-Origin-Opener-Policy", cfg.CrossOriginOpenerPolicy)
			}
			if cfg.CrossOriginResourcePolicy != "" {
				h.Set("Cross-Origin-Resource-Policy", cfg.CrossOriginResourcePolicy)
			}
			if cfg.XXSSProtection != "" {
				h.Set("X-XSS-Protection", cfg.XXSSProtection)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isCompressible(ct string) bool {
	if ct == "" {
		return true // Assume compressible if unknown (e.g. first write)
	}
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/") ||
		strings.Contains(ct, "json") ||
		strings.Contains(ct, "javascript") ||
		strings.Contains(ct, "xml") ||
		strings.Contains(ct, "html")
}

func acceptsEncoding(header, target string) bool {
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		coding, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(coding), target) {
			continue
		}
		if params == "" {
			return true
		}

		allowed := true
		for param := range strings.SplitSeq(params, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || !strings.EqualFold(key, "q") {
				continue
			}
			q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil && q <= 0 {
				allowed = false
			}
		}
		if allowed {
			return true
		}
	}
	return false
}

func addVary(h http.Header, field string) {
	vary := h.Get("Vary")
	for existing := range strings.SplitSeq(vary, ",") {
		if strings.EqualFold(strings.TrimSpace(existing), field) {
			return
		}
	}
	if vary == "" {
		h.Set("Vary", field)
	} else {
		h.Set("Vary", vary+", "+field)
	}
}

func disabledHeaders(headers []string) map[string]bool {
	disabled := make(map[string]bool, len(headers))
	for _, header := range headers {
		if header = strings.TrimSpace(header); header != "" {
			disabled[http.CanonicalHeaderKey(header)] = true
		}
	}
	return disabled
}
