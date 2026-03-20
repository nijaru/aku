package problem

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/nijaru/aku/internal/render"
)

// InvalidParam represents a detailed validation or parsing failure for a single input parameter.
type InvalidParam struct {
	Name   string `json:"name,omitempty"`
	In     string `json:"in,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Details represents an RFC 9457 Problem Details for HTTP APIs.
// It implements the standard error interface.
type Details struct {
	Type          string         `json:"type,omitempty"`
	Title         string         `json:"title,omitempty"`
	Status        int            `json:"status,omitempty"`
	Detail        string         `json:"detail,omitempty"`
	InvalidParams []InvalidParam `json:"invalid_params,omitempty"`
}

// Error implements the error interface.
func (p *Details) Error() string {
	if p.Detail != "" {
		return fmt.Sprintf("[%d] %s: %s", p.Status, p.Title, p.Detail)
	}
	return fmt.Sprintf("[%d] %s", p.Status, p.Title)
}

// BadRequest creates a generic 400 Bad Request problem.
func BadRequest(detail string) *Details {
	return &Details{
		Type:   "about:blank",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

// ValidationProblem creates a 422 Unprocessable Entity problem with validation details.
func ValidationProblem(detail string, params []InvalidParam) *Details {
	return &Details{
		Type:          "https://aku.sh/problems/validation",
		Title:         "Validation failed",
		Status:        http.StatusUnprocessableEntity,
		Detail:        detail,
		InvalidParams: params,
	}
}

// NotFound creates a generic 404 Not Found problem.
func NotFound(detail string) *Details {
	return &Details{
		Type:   "about:blank",
		Title:  "Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
	}
}

// Forbidden creates a generic 403 Forbidden problem.
func Forbidden(detail string) *Details {
	return &Details{
		Type:   "about:blank",
		Title:  "Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
	}
}

// TooManyRequests creates a generic 429 Too Many Requests problem.
func TooManyRequests(detail string) *Details {
	return &Details{
		Type:   "about:blank",
		Title:  "Too Many Requests",
		Status: http.StatusTooManyRequests,
		Detail: detail,
	}
}

// Problemf creates a custom problem with a formatted detail string.
func Problemf(status int, title, format string, args ...any) *Details {
	return &Details{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: fmt.Sprintf(format, args...),
	}
}

// FromValidationErrors converts validator.ValidationErrors to a slice of InvalidParam.
func FromValidationErrors(errs validator.ValidationErrors) []InvalidParam {
	params := make([]InvalidParam, len(errs))
	for i, err := range errs {
		params[i] = InvalidParam{
			Name:   err.Field(),
			Reason: err.Tag(),
		}
		// Try to provide a more descriptive reason for common tags
		switch err.Tag() {
		case "required":
			params[i].Reason = "is required"
		case "email":
			params[i].Reason = "must be a valid email address"
		case "min":
			params[i].Reason = fmt.Sprintf("must be at least %s", err.Param())
		case "max":
			params[i].Reason = fmt.Sprintf("must be at most %s", err.Param())
		}
	}
	return params
}

func defaultErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	prob, ok := errors.AsType[*Details](err)
	if !ok {
		// Wrap unexpected application errors into a 500 Internal Server Error problem.
		prob = Problemf(http.StatusInternalServerError, "Internal Server Error", "%s", err.Error())
	}

	render.Problem(w, prob.Status, prob)
}
