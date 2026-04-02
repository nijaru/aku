package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type Options struct {
	Root       string
	Command    string
	ModulePath string
	Force      bool
}

func Init(opts Options) error {
	if opts.Root == "" {
		opts.Root = "."
	}
	if opts.Command == "" {
		opts.Command = "api"
	}
	if !commandNamePattern.MatchString(opts.Command) {
		return fmt.Errorf(
			"invalid command name %q: use lowercase letters, numbers, and hyphens",
			opts.Command,
		)
	}

	root, err := os.OpenRoot(opts.Root)
	if err != nil {
		return fmt.Errorf("open root: %w", err)
	}
	defer root.Close()

	if opts.ModulePath == "" {
		opts.ModulePath, err = modulePath(root)
		if err != nil {
			return err
		}
	}

	files := map[string]string{
		filepath.Join("cmd", opts.Command, "main.go"):   mainTemplate(opts.ModulePath),
		filepath.Join("internal", "app", "app.go"):      appTemplate(),
		filepath.Join("internal", "app", "app_test.go"): appTestTemplate(opts.ModulePath),
	}

	for rel := range files {
		if _, err := root.Stat(rel); err == nil && !opts.Force {
			return fmt.Errorf("refusing to overwrite existing %s; use --force to replace it", rel)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check %s: %w", rel, err)
		}
	}

	for rel, content := range files {
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(rel), err)
		}
		if err := root.WriteFile(rel, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}

	return nil
}

func modulePath(root *os.Root) (string, error) {
	data, err := root.ReadFile("go.mod")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("go.mod not found; run from a Go module root or pass --module")
		}
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			module := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if module == "" {
				return "", errors.New("go.mod does not declare a module path")
			}
			return module, nil
		}
	}

	return "", errors.New("go.mod does not declare a module path")
}

func mainTemplate(modulePath string) string {
	return fmt.Sprintf(`package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	projectapp "%s/internal/app"
)

func main() {
	addr := ":8080"
	if value := os.Getenv("PORT"); value != "" {
		if _, err := strconv.Atoi(value); err == nil {
			addr = ":" + value
		}
	}

	log.Printf("starting Aku app on %%s", addr)
	log.Fatal(http.ListenAndServe(addr, projectapp.New()))
}
`, modulePath)
}

func appTemplate() string {
	return `package app

import (
	"context"

	"github.com/nijaru/aku"
)

func New() *aku.App {
	app := aku.New()

	aku.Get(app, "/", func(context.Context, struct{}) (map[string]string, error) {
		return map[string]string{"message": "Hello, Aku"}, nil
	})

	app.OpenAPI("/openapi.json", "Aku API", "0.1.0")

	return app
}
`
}

func appTestTemplate(modulePath string) string {
	return fmt.Sprintf(`package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"%s/internal/app"
)

func TestNew(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	app.New().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Hello, Aku") {
		t.Fatalf("expected body to contain %%q, got %%q", "Hello, Aku", w.Body.String())
	}
}
`, modulePath)
}
