package aku

// Binder is the interface that can be implemented by types to customize
// how they are extracted from path, query, or header parameters.
type Binder interface {
	UnmarshalAku(val string) error
}
