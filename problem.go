package aku

import (
	"fmt"
	"net/http"
)

// InvalidParam represents a detailed validation or parsing failure for a single input parameter.
type InvalidParam struct {
	Name   string `json:"name,omitempty"`
	In     string `json:"in,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Problem represents an RFC 9457 Problem Details for HTTP APIs.
// It implements the standard error interface.
type Problem struct {
	Type          string         `json:"type,omitempty"`
	Title         string         `json:"title,omitempty"`
	Status        int            `json:"status,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	InvalidParams []InvalidParam `json:"invalid_params,omitempty"`
}

// Error implements the error interface.
func (p *Problem) Error() string {
	if p.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", p.Status, p.Title, p.Detail)
	}
	return fmt.Sprintf("[%d] %s", p.Status, p.Title)
}

// BadRequest creates a generic 400 Bad Request problem.
func BadRequest(detail string) *Problem {
	return &Problem{
		Type:   "about:blank",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

// ValidationProblem creates a 422 Unprocessable Entity problem with validation details.
func ValidationProblem(detail string, params []InvalidParam) *Problem {
	return &Problem{
		Type:          "https://aku.sh/problems/validation",
		Title:         "Validation failed",
		Status:        http.StatusUnprocessableEntity,
		Detail:        detail,
		InvalidParams: params,
	}
}

// Problemf creates a custom problem with a formatted detail string.
func Problemf(status int, title, format string, args ...any) *Problem {
	return &Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: fmt.Sprintf(format, args...),
	}
}
