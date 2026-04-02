package aku_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/nijaru/aku"
	"github.com/nijaru/aku/internal/testutil"
)

func TestStaticSPA(t *testing.T) {
	app := aku.New()

	spaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(spaDir, "style.css"), []byte("css\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spaDir, "index.html"), []byte("index\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.Static("/app/", spaDir, aku.WithSPA())

	t.Run("Serves existing file", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/app/style.css").
			ExpectStatus(http.StatusOK).
			ExpectBody("css\n")
	})

	t.Run("Fallbacks to index.html for extensionless path", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/app/users/profile").
			ExpectStatus(http.StatusOK).
			ExpectBody("index\n")
	})

	t.Run("Does not fallback for missing file with extension", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/app/missing.js").
			ExpectStatus(http.StatusNotFound)
	})

	t.Run("Serves index.html for root", func(t *testing.T) {
		testutil.Test(t, app).
			Get("/app/").
			ExpectStatus(http.StatusOK).
			ExpectBody("index\n")
	})
}
