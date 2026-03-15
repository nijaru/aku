package bind

import (
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestCoerce(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var v string
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("hello", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "hello" {
			t.Errorf("expected hello, got %s", v)
		}
	})

	t.Run("int", func(t *testing.T) {
		var v int
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("123", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 123 {
			t.Errorf("expected 123, got %d", v)
		}
	})

	t.Run("bool", func(t *testing.T) {
		var v bool
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("true", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != true {
			t.Errorf("expected true, got %v", v)
		}
	})

	t.Run("float64", func(t *testing.T) {
		var v float64
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("12.3", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 12.3 {
			t.Errorf("expected 12.3, got %v", v)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		var v *int
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("456", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v == nil || *v != 456 {
			t.Errorf("expected 456, got %v", v)
		}
	})

	t.Run("double pointer", func(t *testing.T) {
		var v **int
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("789", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v == nil || *v == nil || **v != 789 {
			t.Errorf("expected 789, got %v", v)
		}
	})

	t.Run("time.Time", func(t *testing.T) {
		var v time.Time
		rv := reflect.ValueOf(&v).Elem()
		s := "2026-03-13T10:00:00Z"
		if err := coerce(s, rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected, _ := time.Parse(time.RFC3339, s)
		if !v.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, v)
		}
	})

	t.Run("time.Duration", func(t *testing.T) {
		var v time.Duration
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("5m30s", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != 5*time.Minute+30*time.Second {
			t.Errorf("expected 5m30s, got %v", v)
		}
	})

	t.Run("url.URL", func(t *testing.T) {
		var v url.URL
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("https://example.com/foo?bar=baz", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.String() != "https://example.com/foo?bar=baz" {
			t.Errorf("expected https://example.com/foo?bar=baz, got %s", v.String())
		}
	})

	t.Run("net.IP", func(t *testing.T) {
		var v net.IP
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("192.168.1.1", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.String() != "192.168.1.1" {
			t.Errorf("expected 192.168.1.1, got %s", v.String())
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		var v complex128
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("1+1i", rv); err == nil {
			t.Error("expected error for unsupported type")
		}
	})

	t.Run("custom binder", func(t *testing.T) {
		var v customBinder
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("custom-value", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.val != "custom-value" {
			t.Errorf("expected custom-value, got %s", v.val)
		}
	})

	t.Run("custom binder pointer", func(t *testing.T) {
		var v *customBinder
		rv := reflect.ValueOf(&v).Elem()
		if err := coerce("ptr-custom", rv); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v == nil || v.val != "ptr-custom" {
			t.Errorf("expected ptr-custom, got %v", v)
		}
	})
}

type customBinder struct {
	val string
}

func (c *customBinder) UnmarshalAku(s string) error {
	c.val = s
	return nil
}
