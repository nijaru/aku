package aku_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

type MyInput struct{}
type MyOutput struct{}

func myHandler(ctx context.Context, in MyInput) (MyOutput, error) {
	return MyOutput{}, nil
}

func TestRegister(t *testing.T) {
	app := aku.New()

	err := aku.Get(app, "/test", myHandler, aku.WithStatus(http.StatusCreated))
	if err != nil {
		t.Fatalf("expected nil error on register, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	// Since we haven't implemented the core logic yet, it should return 501
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 Not Implemented, got %d", rr.Code)
	}
}
