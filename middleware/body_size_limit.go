package middleware

import (
	"net/http"
	"strconv"
)

// BodySizeLimitConfig configures the request body size limit middleware.
type BodySizeLimitConfig struct {
	// MaxBodyBytes is the maximum allowed request body size in bytes.
	// Default: 1 MB.
	MaxBodyBytes int64
}

func (c *BodySizeLimitConfig) applyDefaults() {
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = 1 << 20 // 1 MB
	}
}

// BodySizeLimit returns middleware that rejects requests whose body exceeds
// the configured byte limit with a 413 Payload Too Large response.
//
// Two enforcement mechanisms:
//  1. Fast path: if Content-Length is present and exceeds the limit,
//     the request is rejected immediately.
//  2. Enforcement path: the body is wrapped with http.MaxBytesReader for
//     chunked or unknown-size bodies, preventing reads past the limit.
func BodySizeLimit(cfg BodySizeLimitConfig) func(http.Handler) http.Handler {
	cfg.applyDefaults()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > cfg.MaxBodyBytes {
				writeTooLarge(w, cfg.MaxBodyBytes)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
			next.ServeHTTP(w, r)
		})
	}
}

func writeTooLarge(w http.ResponseWriter, limit int64) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	w.Write([]byte(
		`{"type":"about:blank","title":"Payload Too Large",` +
			`"status":` + strconv.Itoa(http.StatusRequestEntityTooLarge) +
			`,"detail":"Request body exceeds maximum allowed size of ` + humanBytes(limit) + `"}`,
	))
}

// humanBytes returns a human-readable byte count (e.g. "10 MB").
func humanBytes(b int64) string {
	const (
		_KB = 1024
		_MB = _KB * 1024
		_GB = _MB * 1024
	)
	switch {
	case b >= _GB:
		return formatNum(b, _GB, "GB")
	case b >= _MB:
		return formatNum(b, _MB, "MB")
	case b >= _KB:
		return formatNum(b, _KB, "KB")
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}

func formatNum(n, unit int64, suffix string) string {
	if n%unit == 0 {
		return strconv.FormatInt(n/unit, 10) + " " + suffix
	}
	quo := n / unit
	rem := (n % unit * 10) / unit
	return strconv.FormatInt(quo, 10) + "." + strconv.FormatInt(rem, 10) + " " + suffix
}
