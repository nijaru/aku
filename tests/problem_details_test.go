package aku_test

import (
	"net/http"
	"testing"

	"github.com/nijaru/aku/problem"
)

func TestProblem_Error(t *testing.T) {
	p := problem.BadRequest("invalid format")

	// Problem must implement error interface
	var _ error = p

	expected := "[400] Bad Request: invalid format"
	if p.Error() != expected {
		t.Errorf("expected %q, got %q", expected, p.Error())
	}
}

func TestValidationProblem(t *testing.T) {
	params := []problem.InvalidParam{
		{Name: "email", In: "body", Reason: "must be a valid email"},
	}
	p := problem.ValidationProblem("Request body failed validation", params)

	if p.Status != http.StatusUnprocessableEntity {
		t.Errorf("expected status %d, got %d", http.StatusUnprocessableEntity, p.Status)
	}
	if len(p.InvalidParams) != 1 {
		t.Errorf("expected 1 invalid param, got %d", len(p.InvalidParams))
	}
}
