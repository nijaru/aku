package aku

import (
	"log/slog"
	"net/http"
	"time"
)

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

		slog.Log(r.Context(), level, "http request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", lw.status),
			slog.Duration("duration", time.Since(start)),
			slog.Int("size", lw.size),
			slog.String("ip", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
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
