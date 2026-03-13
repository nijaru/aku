package bind

import (
	"context"
	"net/http"
	"reflect"
)

// Extractor is a precompiled function that populates an input struct
// from an HTTP request.
type Extractor func(context.Context, *http.Request, reflect.Value) error

// Compiler inspects a generic input type once at startup and builds
// a static Extractor that avoids per-request reflection overhead.
func Compiler[T any]() Extractor {
	var t T
	typ := reflect.TypeOf(t)

	// If the input is not a struct, or is empty, we don't need to extract anything.
	if typ == nil || typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error {
			return nil
		}
	}

	// We'll build up a list of step functions that each handle one section
	// (Path, Query, Header, Body) and execute them in order at runtime.
	var steps []Extractor

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// For MVP, we only care about specific exported top-level struct sections.
		switch field.Name {
		case "Path":
			steps = append(steps, compilePath(i, field.Type))
		case "Query":
			steps = append(steps, compileQuery(i, field.Type))
		case "Header":
			steps = append(steps, compileHeader(i, field.Type))
		case "Body":
			steps = append(steps, compileBody(i, field.Type))
		}
	}

	// The returned Extractor simply runs all the compiled steps.
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		for _, step := range steps {
			if err := step(ctx, r, v); err != nil {
				return err
			}
		}
		return nil
	}
}

// compilePath creates an Extractor for the Path section of the request struct.
func compilePath(fieldIdx int, typ reflect.Type) Extractor {
	// TODO: Parse `path:"name"` tags and map to r.PathValue("name")
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		return nil
	}
}

// compileQuery creates an Extractor for the Query section of the request struct.
func compileQuery(fieldIdx int, typ reflect.Type) Extractor {
	// TODO: Parse `query:"name"` tags and map to r.URL.Query().Get("name")
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		return nil
	}
}

// compileHeader creates an Extractor for the Header section of the request struct.
func compileHeader(fieldIdx int, typ reflect.Type) Extractor {
	// TODO: Parse `header:"Name"` tags and map to r.Header.Get("Name")
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		return nil
	}
}

// compileBody creates an Extractor for the Body section of the request struct.
func compileBody(fieldIdx int, typ reflect.Type) Extractor {
	// TODO: JSON decode r.Body into the struct field
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		return nil
	}
}
