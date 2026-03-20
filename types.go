package aku

import (
	"io"
	"net/http"

	"github.com/nijaru/aku/internal/bind"
)

// SecurityScheme describes an authentication scheme for the API.
type SecurityScheme struct {
	Type             string
	Description      string
	Name             string // for apiKey
	In               string // for apiKey: "query", "header", "cookie"
	Scheme           string // for http
	BearerFormat     string // for http ("bearer")
	OpenIdConnectUrl string // for openIdConnect
}

// Validator is the interface that wraps the basic Validate method.
type Validator interface {
	Struct(s any) error
}

// ErrorHandler is a function that handles errors returned by handlers or the framework.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// ContextKey is a custom type used for context value lookups to avoid collisions
// with built-in string types, satisfying standard go linters (e.g., SA1029).
type ContextKey = bind.ContextKey

// Binder is the interface that can be implemented by types to customize
// how they are extracted from path, query, or header parameters.
type Binder interface {
	UnmarshalAku(val string) error
}

// Stream represents a streaming response.
type Stream struct {
	Reader      io.Reader
	ContentType string
}

// Event represents a single Server-Sent Event.
type Event struct {
	ID    string
	Event string
	Data  any
}

// SSE represents a Server-Sent Events response.
type SSE struct {
	Events <-chan Event
}
