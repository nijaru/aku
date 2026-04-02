package aku_test

import "net/http"

// benchmarkResponseWriter discards response bodies while preserving header and status behavior.
type benchmarkResponseWriter struct {
	header http.Header
	status int
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{header: make(http.Header)}
}

func (w *benchmarkResponseWriter) reset() {
	clear(w.header)
	w.status = 0
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *benchmarkResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	return len(p), nil
}
