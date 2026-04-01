package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodySizeLimit_Allowed(t *testing.T) {
	mw := BodySizeLimit(BodySizeLimitConfig{MaxBodyBytes: 1024})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, 500)))
	req.ContentLength = 500
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestBodySizeLimit_Rejected_ByContentLength(t *testing.T) {
	mw := BodySizeLimit(BodySizeLimitConfig{MaxBodyBytes: 1024})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, 2048)))
	req.ContentLength = 2048
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `Payload Too Large`) {
		t.Fatalf("expected problem JSON, got: %s", body)
	}
	if !strings.Contains(body, `1 KB`) {
		t.Fatalf("expected '1 KB' in detail, got: %s", body)
	}
}

func TestBodySizeLimit_Rejected_ExactlyAtLimit(t *testing.T) {
	mw := BodySizeLimit(BodySizeLimitConfig{MaxBodyBytes: 1024})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exactly at limit should be allowed
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, 1024)))
	req.ContentLength = 1024
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 at exact limit, got %d", rec.Code)
	}
}

func TestBodySizeLimit_DefaultConfig(t *testing.T) {
	// Empty config should default to 1 MB
	mw := BodySizeLimit(BodySizeLimitConfig{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 900 KB should pass
	body := make([]byte, 900*1024)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for 900 KB with 1 MB default, got %d", rec.Code)
	}

	// 2 MB should fail
	body = make([]byte, 2*1024*1024)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	rec = httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for 2 MB with 1 MB default, got %d", rec.Code)
	}
}

func TestBodySizeLimit_HumanBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1536, "1.5 KB"},
		{1048576, "1 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("humanBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestBodySizeLimit_GetRequests(t *testing.T) {
	// GET requests with no body should pass through
	mw := BodySizeLimit(BodySizeLimitConfig{MaxBodyBytes: 1024})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d", rec.Code)
	}
}
