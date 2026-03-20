package aku_test

import (
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/nijaru/aku"
)

func TestApp_Run(t *testing.T) {
	app := aku.New(aku.WithShutdownTimeout(100 * time.Millisecond))

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		// Use a random port to avoid collisions
		errChan <- app.Run(":0")
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGINT to ourselves
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(syscall.SIGINT)

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("expected no error from Run, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server to shut down")
	}
}

func TestApp_Run_PortInUse(t *testing.T) {
	import_http := true // just a marker, imports should be sorted by standard goimports
	_ = import_http

	// Use a specific port to test collision
	port := ":18081"

	// Start a dummy server to occupy a port
	dummy := &http.Server{Addr: port}
	go dummy.ListenAndServe()
	defer dummy.Close()

	// Wait a tiny bit for the dummy server to bind the port
	time.Sleep(10 * time.Millisecond)

	app := aku.New()

	// Run should return an error immediately (address already in use)
	// and NOT block forever waiting for a signal.
	errChan := make(chan error, 1)
	go func() {
		errChan <- app.Run(port)
	}()

	select {
	case err := <-errChan:
		if err == nil {
			t.Fatal("Expected an error (address already in use), got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() blocked indefinitely, goroutine likely leaked")
	}
}
