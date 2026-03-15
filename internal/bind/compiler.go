package bind

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
)

// Config holds configuration for the binder extractors.
type Config struct {
	MaxMultipartMemory int64
}

// Extractor is a precompiled function that populates an input struct
// from an HTTP request.
type Extractor func(context.Context, *http.Request, reflect.Value, *Config) error

// Schema describes the structure of an input type for documentation purposes.
type Schema struct {
	Parameters []Parameter
	Body       reflect.Type
}

// Parameter describes a single input parameter from the path, query, or headers.
type Parameter struct {
	Name     string
	In       string // "path", "query", "header", "form"
	Type     reflect.Type
	Required bool
	Validate string
}

// BindError represents an error that occurred during request extraction or validation.
type BindError struct {
	Field  string
	Source string // "path", "query", "header", "body"
	Err    error
}

func (e *BindError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s %q: %v", e.Source, e.Field, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Source, e.Err)
}

func (e *BindError) Unwrap() error {
	return e.Err
}

// Compiler inspects a generic input type once at startup and builds
// a static Extractor and Schema that avoids per-request reflection overhead.
func Compiler[T any]() (Extractor, *Schema) {
	var t T
	typ := reflect.TypeOf(t)
	schema := &Schema{}

	// If the input is not a struct, or is empty, we don't need to extract anything.
	if typ == nil || typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
			return nil
		}, schema
	}

	// Build up a list of step functions that each handle one section
	// (Path, Query, Header, Body) and execute them in order at runtime.
	var steps []Extractor

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// For MVP, we only care about specific exported top-level struct sections.
		switch field.Name {
		case "Path":
			ex, params := compilePath(i, field.Type)
			steps = append(steps, ex)
			schema.Parameters = append(schema.Parameters, params...)
		case "Query":
			ex, params := compileQuery(i, field.Type)
			steps = append(steps, ex)
			schema.Parameters = append(schema.Parameters, params...)
		case "Header":
			ex, params := compileHeader(i, field.Type)
			steps = append(steps, ex)
			schema.Parameters = append(schema.Parameters, params...)
		case "Form":
			ex, params := compileForm(i, field.Type)
			steps = append(steps, ex)
			schema.Parameters = append(schema.Parameters, params...)
		case "Body":
			steps = append(steps, compileBody(i, field.Type))
			schema.Body = field.Type
		}
	}

	// If the type implements interface{ Validate() error }, call it after extraction.
	var validator func(reflect.Value) error
	if validateMethod, ok := reflect.PointerTo(typ).MethodByName("Validate"); ok {
		if validateMethod.Type.NumIn() == 1 && validateMethod.Type.NumOut() == 1 && validateMethod.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			validator = func(v reflect.Value) error {
				out := validateMethod.Func.Call([]reflect.Value{v.Addr()})
				if !out[0].IsNil() {
					return out[0].Interface().(error)
				}
				return nil
			}
		}
	}

	// The returned Extractor simply runs all the compiled steps.
	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		for _, step := range steps {
			if err := step(ctx, r, v, cfg); err != nil {
				return err
			}
		}
		if validator != nil {
			if err := validator(v); err != nil {
				return err
			}
		}
		return nil
	}, schema
}

type fieldInfo struct {
	idx     int
	name    string
	isSlice bool
	isMap   bool
}
