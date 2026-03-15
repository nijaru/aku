package bind

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// Extractor is a precompiled function that populates an input struct
// from an HTTP request.
type Extractor func(context.Context, *http.Request, reflect.Value) error

// Schema describes the structure of an input type for documentation purposes.
type Schema struct {
	Parameters []Parameter
	Body       reflect.Type
}

// Parameter describes a single input parameter from the path, query, or headers.
type Parameter struct {
	Name     string
	In       string // "path", "query", "header"
	Type     reflect.Type
	Required bool
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
		return func(ctx context.Context, r *http.Request, v reflect.Value) error {
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
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		for _, step := range steps {
			if err := step(ctx, r, v); err != nil {
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

// compilePath creates an Extractor for the Path section of the request struct.
func compilePath(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error { return nil }, nil
	}

	var infos []fieldInfo
	var params []Parameter
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("path")
		if tag != "" {
			infos = append(infos, fieldInfo{idx: i, name: tag})
			params = append(params, Parameter{
				Name:     tag,
				In:       "path",
				Type:     field.Type,
				Required: field.Type.Kind() != reflect.Pointer,
			})
		}
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		section := v.Field(sectionIdx)
		for _, info := range infos {
			val := r.PathValue(info.name)
			if val != "" {
				if err := coerce(val, section.Field(info.idx)); err != nil {
					return &BindError{Field: info.name, Source: "path", Err: err}
				}
			}
		}
		return nil
	}, params
}

// compileQuery creates an Extractor for the Query section of the request struct.
func compileQuery(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error { return nil }, nil
	}

	var infos []fieldInfo
	var params []Parameter
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("query")
		if tag != "" {
			infos = append(infos, fieldInfo{
				idx:     i,
				name:    tag,
				isSlice: field.Type.Kind() == reflect.Slice,
				isMap:   field.Type.Kind() == reflect.Map,
			})
			params = append(params, Parameter{
				Name:     tag,
				In:       "query",
				Type:     field.Type,
				Required: field.Type.Kind() != reflect.Pointer,
			})
		}
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		section := v.Field(sectionIdx)
		query := r.URL.Query()
		for _, info := range infos {
			f := section.Field(info.idx)
			if info.isSlice {
				vals := query[info.name]
				if len(vals) > 0 {
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := coerce(val, slice.Index(i)); err != nil {
							return &BindError{Field: info.name, Source: "query", Err: err}
						}
					}
					f.Set(slice)
				}
			} else if info.isMap {
				// Support name[key]=val pattern for maps
				prefix := info.name + "["
				m := reflect.MakeMap(f.Type())
				found := false
				for k, vals := range query {
					if len(k) > len(prefix)+1 && k[:len(prefix)] == prefix && k[len(k)-1] == ']' {
						key := k[len(prefix) : len(k)-1]
						val := vals[0] // take first for map
						valVal := reflect.New(f.Type().Elem()).Elem()
						if err := coerce(val, valVal); err != nil {
							return &BindError{Field: info.name + "[" + key + "]", Source: "query", Err: err}
						}
						m.SetMapIndex(reflect.ValueOf(key), valVal)
						found = true
					}
				}
				if found {
					f.Set(m)
				}
			} else {
				val := query.Get(info.name)
				if val != "" {
					if err := coerce(val, f); err != nil {
						return &BindError{Field: info.name, Source: "query", Err: err}
					}
				}
			}
		}
		return nil
	}, params
}

// compileHeader creates an Extractor for the Header section of the request struct.
func compileHeader(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error { return nil }, nil
	}

	var infos []fieldInfo
	var params []Parameter
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("header")
		if tag != "" {
			infos = append(infos, fieldInfo{
				idx:     i,
				name:    tag,
				isSlice: field.Type.Kind() == reflect.Slice,
				isMap:   field.Type.Kind() == reflect.Map,
			})
			params = append(params, Parameter{
				Name:     tag,
				In:       "header",
				Type:     field.Type,
				Required: field.Type.Kind() != reflect.Pointer,
			})
		}
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		section := v.Field(sectionIdx)
		for _, info := range infos {
			f := section.Field(info.idx)
			if info.isSlice {
				vals := r.Header[info.name]
				if len(vals) > 0 {
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := coerce(val, slice.Index(i)); err != nil {
							return &BindError{Field: info.name, Source: "header", Err: err}
						}
					}
					f.Set(slice)
				}
			} else if info.isMap {
				// For headers, we support prefix matching if the tag ends with -
				// or name[key] style if it's a standard map name.
				isPrefix := info.name[len(info.name)-1] == '-'
				m := reflect.MakeMap(f.Type())
				found := false
				for k, vals := range r.Header {
					if isPrefix {
						if len(k) > len(info.name) && k[:len(info.name)] == info.name {
							key := k[len(info.name):]
							val := vals[0]
							valVal := reflect.New(f.Type().Elem()).Elem()
							if err := coerce(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(reflect.ValueOf(key), valVal)
							found = true
						}
					} else {
						prefix := info.name + "["
						if len(k) > len(prefix)+1 && k[:len(prefix)] == prefix && k[len(k)-1] == ']' {
							key := k[len(prefix) : len(k)-1]
							val := vals[0]
							valVal := reflect.New(f.Type().Elem()).Elem()
							if err := coerce(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(reflect.ValueOf(key), valVal)
							found = true
						}
					}
				}
				if found {
					f.Set(m)
				}
			} else {
				val := r.Header.Get(info.name)
				if val != "" {
					if err := coerce(val, f); err != nil {
						return &BindError{Field: info.name, Source: "header", Err: err}
					}
				}
			}
		}
		return nil
	}, params
}

// compileBody creates an Extractor for the Body section of the request struct.
func compileBody(sectionIdx int, typ reflect.Type) Extractor {
	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		if r.Body == nil || r.ContentLength == 0 {
			return nil
		}
		section := v.Field(sectionIdx)
		if err := json.NewDecoder(r.Body).Decode(section.Addr().Interface()); err != nil {
			return &BindError{Source: "body", Err: err}
		}
		return nil
	}
}
