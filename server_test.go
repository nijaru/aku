package aku

import (
	"testing"
	"time"
)

func TestNew_DefaultServerTimeouts(t *testing.T) {
	app := New()
	srv := app.server(":0")

	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected ReadHeaderTimeout=5s, got %s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Fatalf("expected ReadTimeout=30s, got %s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Fatalf("expected WriteTimeout=30s, got %s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Fatalf("expected IdleTimeout=120s, got %s", srv.IdleTimeout)
	}
}

func TestNew_CustomServerTimeouts(t *testing.T) {
	app := New(WithServerTimeouts(ServerTimeouts{
		ReadHeader: time.Second,
		Read:       2 * time.Second,
		Write:      3 * time.Second,
		Idle:       4 * time.Second,
	}))
	srv := app.server(":0")

	if srv.ReadHeaderTimeout != time.Second {
		t.Fatalf("expected ReadHeaderTimeout=1s, got %s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 2*time.Second {
		t.Fatalf("expected ReadTimeout=2s, got %s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 3*time.Second {
		t.Fatalf("expected WriteTimeout=3s, got %s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 4*time.Second {
		t.Fatalf("expected IdleTimeout=4s, got %s", srv.IdleTimeout)
	}
}
