package aku_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nijaru/aku"
	"github.com/nijaru/aku/middleware"
)

func TestCompressionMiddleware(t *testing.T) {
	app := aku.New(aku.WithGlobalMiddleware(middleware.Compress))

	type In struct{}
	type Out struct {
		Message string `json:"message"`
	}

	h := func(ctx context.Context, in In) (Out, error) {
		return Out{Message: strings.Repeat("a", 2000)}, nil
	}

	aku.Get(app, "/test", h)

	t.Run("Brotli", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected Content-Encoding br, got %s", rec.Header().Get("Content-Encoding"))
		}

		br := brotli.NewReader(rec.Body)
		data, err := io.ReadAll(br)
		if err != nil {
			t.Fatal(err)
		}

		expected := `{"message":"` + strings.Repeat("a", 2000) + `"}
`
		if string(data) != expected {
			t.Errorf("expected len %d, got len %d", len(expected), len(data))
		}
	})

	t.Run("Zstd", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "zstd")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Encoding") != "zstd" {
			t.Errorf("expected Content-Encoding zstd, got %s", rec.Header().Get("Content-Encoding"))
		}

		zr, err := zstd.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()

		data, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}

		expected := `{"message":"` + strings.Repeat("a", 2000) + `"}
`
		if string(data) != expected {
			t.Errorf("expected len %d, got len %d", len(expected), len(data))
		}
	})

	t.Run("Gzip", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("expected Content-Encoding gzip, got %s", rec.Header().Get("Content-Encoding"))
		}

		gr, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer gr.Close()

		data, err := io.ReadAll(gr)
		if err != nil {
			t.Fatal(err)
		}

		expected := `{"message":"` + strings.Repeat("a", 2000) + `"}
`
		if string(data) != expected {
			t.Errorf("expected len %d, got len %d", len(expected), len(data))
		}
	})

	t.Run("NoCompression", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected no Content-Encoding, got %s", rec.Header().Get("Content-Encoding"))
		}

		expected := `{"message":"` + strings.Repeat("a", 2000) + `"}
`
		if rec.Body.String() != expected {
			t.Errorf("expected len %d, got len %d", len(expected), rec.Body.Len())
		}
	})

	t.Run("NonCompressibleType", func(t *testing.T) {
		// Register a route that returns an image (mocked)
		aku.Get(app, "/image", func(ctx context.Context, in In) (aku.Stream, error) {
			return aku.Stream{
				Reader:      bytes.NewReader([]byte("fake-image-data")),
				ContentType: "image/png",
			}, nil
		})

		req := httptest.NewRequest("GET", "/image", nil)
		req.Header.Set("Accept-Encoding", "br")
		rec := httptest.NewRecorder()

		app.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected no Content-Encoding for image/png, got %s", rec.Header().Get("Content-Encoding"))
		}
		if rec.Body.String() != "fake-image-data" {
			t.Errorf("expected fake-image-data, got %s", rec.Body.String())
		}
	})
}
