package aku_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nijaru/aku"
)

func TestStream_Reader(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/stream", func(ctx context.Context, in any) (io.Reader, error) {
		return strings.NewReader("hello world"), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	if rr.Body.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", rr.Body.String())
	}

	// Default content type for io.Reader should probably be application/octet-stream
	// unless we find a better way.
	if rr.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %q", rr.Header().Get("Content-Type"))
	}
}

func TestStream_StreamType(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/stream-type", func(ctx context.Context, in any) (aku.Stream, error) {
		return aku.Stream{
			Reader:      strings.NewReader("hello text"),
			ContentType: "text/plain",
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/stream-type", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("expected text/plain, got %q", rr.Header().Get("Content-Type"))
	}

	if rr.Body.String() != "hello text" {
		t.Errorf("expected 'hello text', got %q", rr.Body.String())
	}
}

func TestStream_SSE(t *testing.T) {
	app := aku.New()

	aku.Get(app, "/events", func(ctx context.Context, in any) (aku.SSE, error) {
		ch := make(chan aku.Event, 2)
		ch <- aku.Event{Data: "hello"}
		ch <- aku.Event{Data: "world"}
		close(ch)
		return aku.SSE{Events: ch}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %q", rr.Header().Get("Content-Type"))
	}

	expected := "data: hello\n\ndata: world\n\n"
	if rr.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, rr.Body.String())
	}
}
