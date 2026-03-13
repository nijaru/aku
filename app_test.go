package aku_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nijaru/aku"
)

func TestApp_ServeHTTP(t *testing.T) {
	app := aku.New()

	// App should satisfy the http.Handler interface
	var _ http.Handler = app

	req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", rr.Code)
	}
}
