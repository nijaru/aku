package aku_test

import (
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
