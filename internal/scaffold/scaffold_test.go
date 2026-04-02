package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.26.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(Options{Root: root, Command: "api"}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, "cmd", "api", "main.go"),
		filepath.Join(root, "internal", "app", "app.go"),
		filepath.Join(root, "internal", "app", "app_test.go"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestInitRefusesToOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.26.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "api", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(Options{Root: root, Command: "api"}); err == nil {
		t.Fatal("expected Init() to refuse overwriting existing files")
	}
}
