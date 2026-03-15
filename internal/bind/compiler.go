package bind

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"time"
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
				Validate: field.Tag.Get("validate"),
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

	steps, params := compileQueryLevel(typ, "")

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		section := v.Field(sectionIdx)
		query := r.URL.Query()
		for _, step := range steps {
			if err := step(query, section); err != nil {
				return err
			}
		}
		return nil
	}, params
}

type queryStep func(url.Values, reflect.Value) error

func compileQueryLevel(typ reflect.Type, prefix string) ([]queryStep, []Parameter) {
	var steps []queryStep
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("query")
		if tag == "" {
			continue
		}

		name := tag
		if prefix != "" {
			name = prefix + "[" + tag + "]"
		}

		fTyp := field.Type
		for fTyp.Kind() == reflect.Pointer {
			fTyp = fTyp.Elem()
		}

		// Support recursion for structs that are not Custom Binders
		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeOf(time.Time{}) && !fTyp.Implements(binderType) && !reflect.PointerTo(fTyp).Implements(binderType) {
			subSteps, subParams := compileQueryLevel(fTyp, name)
			subPrefix := name + "["
			steps = append(steps, func(q url.Values, v reflect.Value) error {
				// Only allocate/recurse if there's actually data for this struct
				found := false
				for k := range q {
					if len(k) > len(subPrefix) && k[:len(subPrefix)] == subPrefix {
						found = true
						break
					}
				}
				if !found {
					return nil
				}

				f := v.Field(i)
				if f.Kind() == reflect.Pointer {
					if f.IsNil() {
						f.Set(reflect.New(f.Type().Elem()))
					}
					f = f.Elem()
				}
				for _, subStep := range subSteps {
					if err := subStep(q, f); err != nil {
						return err
					}
				}
				return nil
			})
			params = append(params, subParams...)
			continue
		}

		// Leaf fields (or slices/maps/binders)
		isSlice := field.Type.Kind() == reflect.Slice
		isMap := field.Type.Kind() == reflect.Map
		fieldIdx := i
		fieldName := name

		steps = append(steps, func(query url.Values, v reflect.Value) error {
			f := v.Field(fieldIdx)
			if isSlice {
				vals := query[fieldName]
				if len(vals) > 0 {
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := coerce(val, slice.Index(i)); err != nil {
							return &BindError{Field: fieldName, Source: "query", Err: err}
						}
					}
					f.Set(slice)
				}
			} else if isMap {
				// Support name[key]=val pattern for maps
				prefix := fieldName + "["
				m := reflect.MakeMap(f.Type())
				found := false
				for k, vals := range query {
					if len(k) > len(prefix)+1 && k[:len(prefix)] == prefix && k[len(k)-1] == ']' {
						key := k[len(prefix) : len(k)-1]
						val := vals[0] // take first for map
						valVal := reflect.New(f.Type().Elem()).Elem()
						if err := coerce(val, valVal); err != nil {
							return &BindError{Field: fieldName + "[" + key + "]", Source: "query", Err: err}
						}
						m.SetMapIndex(reflect.ValueOf(key), valVal)
						found = true
					}
				}
				if found {
					f.Set(m)
				}
			} else {
				val := query.Get(fieldName)
				if val != "" {
					if err := coerce(val, f); err != nil {
						return &BindError{Field: fieldName, Source: "query", Err: err}
					}
				}
			}
			return nil
		})

		params = append(params, Parameter{
			Name:     name,
			In:       "query",
			Type:     field.Type,
			Required: field.Type.Kind() != reflect.Pointer,
			Validate: field.Tag.Get("validate"),
		})
	}

	return steps, params
}

// compileHeader creates an Extractor for the Header section of the request struct.
func compileHeader(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error { return nil }, nil
	}

	steps, params := compileHeaderLevel(typ, "")

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		section := v.Field(sectionIdx)
		for _, step := range steps {
			if err := step(r.Header, section); err != nil {
				return err
			}
		}
		return nil
	}, params
}

type headerStep func(http.Header, reflect.Value) error

func compileHeaderLevel(typ reflect.Type, prefix string) ([]headerStep, []Parameter) {
	var steps []headerStep
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("header")
		if tag == "" {
			continue
		}

		name := tag
		if prefix != "" {
			name = prefix + "[" + tag + "]"
		}

		fTyp := field.Type
		for fTyp.Kind() == reflect.Pointer {
			fTyp = fTyp.Elem()
		}

		// Support recursion for structs that are not Custom Binders
		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeOf(time.Time{}) && !fTyp.Implements(binderType) && !reflect.PointerTo(fTyp).Implements(binderType) {
			subSteps, subParams := compileHeaderLevel(fTyp, name)
			subPrefix := name + "["
			steps = append(steps, func(h http.Header, v reflect.Value) error {
				// Only allocate/recurse if there's actually data for this struct
				found := false
				for k := range h {
					if len(k) > len(subPrefix) && k[:len(subPrefix)] == subPrefix {
						found = true
						break
					}
				}
				if !found {
					return nil
				}

				f := v.Field(i)
				if f.Kind() == reflect.Pointer {
					if f.IsNil() {
						f.Set(reflect.New(f.Type().Elem()))
					}
					f = f.Elem()
				}
				for _, subStep := range subSteps {
					if err := subStep(h, f); err != nil {
						return err
					}
				}
				return nil
			})
			params = append(params, subParams...)
			continue
		}

		isSlice := field.Type.Kind() == reflect.Slice
		isMap := field.Type.Kind() == reflect.Map
		fieldIdx := i
		fieldName := name

		steps = append(steps, func(header http.Header, v reflect.Value) error {
			f := v.Field(fieldIdx)
			if isSlice {
				vals := header[fieldName]
				if len(vals) > 0 {
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := coerce(val, slice.Index(i)); err != nil {
							return &BindError{Field: fieldName, Source: "header", Err: err}
						}
					}
					f.Set(slice)
				}
			} else if isMap {
				// For headers, we support prefix matching if the tag ends with -
				// or name[key] style if it's a standard map name.
				isPrefix := fieldName[len(fieldName)-1] == '-'
				m := reflect.MakeMap(f.Type())
				found := false
				for k, vals := range header {
					if isPrefix {
						if len(k) > len(fieldName) && k[:len(fieldName)] == fieldName {
							key := k[len(fieldName):]
							val := vals[0]
							valVal := reflect.New(f.Type().Elem()).Elem()
							if err := coerce(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(reflect.ValueOf(key), valVal)
							found = true
						}
					} else {
						mapPrefix := fieldName + "["
						if len(k) > len(mapPrefix)+1 && k[:len(mapPrefix)] == mapPrefix && k[len(k)-1] == ']' {
							key := k[len(mapPrefix) : len(k)-1]
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
				val := header.Get(fieldName)
				if val != "" {
					if err := coerce(val, f); err != nil {
						return &BindError{Field: fieldName, Source: "header", Err: err}
					}
				}
			}
			return nil
		})

		params = append(params, Parameter{
			Name:     name,
			In:       "header",
			Type:     field.Type,
			Required: field.Type.Kind() != reflect.Pointer,
			Validate: field.Tag.Get("validate"),
		})
	}

	return steps, params
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

// compileForm creates an Extractor for the Form section of the request struct.
func compileForm(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value) error { return nil }, nil
	}

	var infos []fieldInfo
	var fileInfos []fieldInfo
	var params []Parameter

	fileHeaderType := reflect.TypeOf((*multipart.FileHeader)(nil))
	fileHeaderSliceType := reflect.TypeOf([]*multipart.FileHeader(nil))

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("form")
		if tag == "" {
			continue
		}

		if field.Type == fileHeaderType || field.Type == fileHeaderSliceType {
			fileInfos = append(fileInfos, fieldInfo{idx: i, name: tag, isSlice: field.Type.Kind() == reflect.Slice})
		} else {
			infos = append(infos, fieldInfo{idx: i, name: tag, isSlice: field.Type.Kind() == reflect.Slice})
		}

		params = append(params, Parameter{
			Name:     tag,
			In:       "form",
			Type:     field.Type,
			Required: field.Type.Kind() != reflect.Pointer,
			Validate: field.Tag.Get("validate"),
		})
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value) error {
		// Ensure form is parsed.
		// Use a reasonable default max memory for now.
		if err := r.ParseMultipartForm(32 << 20); err != nil && err != http.ErrNotMultipart {
			return &BindError{Source: "form", Err: err}
		}

		section := v.Field(sectionIdx)

		// Regular form values
		for _, info := range infos {
			if info.isSlice {
				vals := r.Form[info.name]
				if len(vals) > 0 {
					f := section.Field(info.idx)
					slice := reflect.MakeSlice(f.Type(), len(vals), len(vals))
					for i, val := range vals {
						if err := coerce(val, slice.Index(i)); err != nil {
							return &BindError{Field: info.name, Source: "form", Err: err}
						}
					}
					f.Set(slice)
				}
			} else {
				val := r.FormValue(info.name)
				if val != "" {
					if err := coerce(val, section.Field(info.idx)); err != nil {
						return &BindError{Field: info.name, Source: "form", Err: err}
					}
				}
			}
		}

		// Multipart files
		if r.MultipartForm != nil {
			for _, info := range fileInfos {
				files := r.MultipartForm.File[info.name]
				if len(files) > 0 {
					f := section.Field(info.idx)
					if info.isSlice {
						slice := reflect.MakeSlice(f.Type(), len(files), len(files))
						for i, fh := range files {
							slice.Index(i).Set(reflect.ValueOf(fh))
						}
						f.Set(slice)
					} else {
						f.Set(reflect.ValueOf(files[0]))
					}
				}
			}
		}

		return nil
	}, params
}
