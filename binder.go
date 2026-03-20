package aku

import "github.com/nijaru/aku/internal/bind"

// ContextKey is a custom type used for context value lookups to avoid collisions
// with built-in string types, satisfying standard go linters (e.g., SA1029).
type ContextKey = bind.ContextKey

// Binder is the interface that can be implemented by types to customize
// how they are extracted from path, query, or header parameters.
type Binder interface {
	UnmarshalAku(val string) error
}
