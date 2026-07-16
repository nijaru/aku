package bind

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// Config holds configuration for the binder extractors.
type Config struct {
	MaxMultipartMemory int64
	StrictQuery        bool
	StrictHeader       bool
}

// Extractor is a precompiled function that populates an input struct
// from an HTTP request.
type Extractor[T any] func(context.Context, *http.Request, *T, reflect.Value, *Config) error

// internalExtractor is used internally by the compiler to bind sections.
type internalExtractor func(context.Context, *http.Request, reflect.Value, *Config) error

// Schema describes the structure of an input type for documentation purposes.
type Schema struct {
	Parameters []Parameter
	Body       reflect.Type
	Auth       []AuthScheme
}

// AuthScheme describes an authentication scheme for OpenAPI documentation
type AuthScheme struct {
	Name        string // e.g. "bearerAuth", "ApiKeyAuth"
	Type        string // "http" | "apiKey"
	Scheme      string // For http: "bearer"
	In          string // For apiKey: "header" | "query"
	ParamName   string // For apiKey: the header or query param name
	BearerFmt   string // For http bearer: "JWT"
	Required    bool
	Description string
}

// Parameter describes a single input parameter from the path, query, or headers.
type Parameter struct {
	Name     string
	In       string // "path", "query", "header", "form"
	Type     reflect.Type
	Required bool
	Validate string
	Message  string
	Example  string
}

// BindError represents an error that occurred during request extraction or validation.
type BindError struct {
	Field     string
	Source    string // "path", "query", "header", "body"
	Err       error
	Challenge string // Optional WWW-Authenticate challenge for auth failures.
}

// Validate checks the input shape and tagged scalar types before a route is
// registered. Keeping this separate from Compiler preserves the small test and
// internal compiler API while allowing the public router to fail fast on
// misconfigured routes instead of returning the same 422 for every request.
func Validate[T any]() error {
	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Interface {
		return nil
	}
	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("input type must be a struct or interface, got %s", typ)
	}

	hasBody := false
	hasForm := false
	for field := range typ.Fields() {
		if field.PkgPath != "" {
			continue
		}

		switch field.Name {
		case "Path":
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
			if err := validateTaggedFields(field.Type, "path", field.Name); err != nil {
				return err
			}
		case "Query":
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
			if err := validateTaggedFields(field.Type, "query", field.Name); err != nil {
				return err
			}
		case "Header":
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
			if err := validateTaggedFields(field.Type, "header", field.Name); err != nil {
				return err
			}
		case "Form":
			hasForm = true
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
			if err := validateTaggedFields(field.Type, "form", field.Name); err != nil {
				return err
			}
		case "Body":
			hasBody = true
		case "Ctx":
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
		case "Auth":
			if field.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%s section must be a struct, got %s", field.Name, field.Type)
			}
			if err := validateAuthFields(field.Type); err != nil {
				return err
			}
		default:
			continue
		}
	}
	if hasBody && hasForm {
		return errors.New("input cannot declare both Body and Form sections")
	}
	return nil
}

func validateTaggedFields(typ reflect.Type, tagName, prefix string) error {
	return validateTaggedFieldsStack(typ, tagName, prefix, make(map[reflect.Type]bool))
}

func validateTaggedFieldsStack(
	typ reflect.Type,
	tagName, prefix string,
	stack map[reflect.Type]bool,
) error {
	if typ.Kind() != reflect.Struct {
		return nil
	}
	if stack[typ] {
		return fmt.Errorf("recursive %s binding through %s", tagName, prefix)
	}
	stack[typ] = true
	defer delete(stack, typ)

	fileHeaderType := reflect.TypeFor[*multipart.FileHeader]()
	fileHeaderSliceType := reflect.TypeFor[[]*multipart.FileHeader]()
	for field := range typ.Fields() {
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get(tagName)
		if tag == "" {
			continue
		}

		name := prefix + "." + field.Name
		if tagName == "form" &&
			(field.Type == fileHeaderType || field.Type == fileHeaderSliceType) {
			continue
		}

		base := field.Type
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		isBinder := base.Implements(binderType) || reflect.PointerTo(base).Implements(binderType)
		isText := base.Implements(textUnmarshalerType) ||
			reflect.PointerTo(base).Implements(textUnmarshalerType)
		if base.Kind() == reflect.Struct && base != reflect.TypeFor[time.Time]() && !isBinder &&
			!isText {
			if err := validateTaggedFieldsStack(base, tagName, name, stack); err != nil {
				return err
			}
			continue
		}

		if field.Type.Kind() == reflect.Slice {
			if err := validateCoercer(field.Type.Elem()); err != nil {
				return fmt.Errorf("%s (%s): %w", name, tag, err)
			}
			continue
		}
		if field.Type.Kind() == reflect.Map {
			if err := validateCoercer(field.Type.Key()); err != nil {
				return fmt.Errorf("%s (%s) map key: %w", name, tag, err)
			}
			if err := validateCoercer(field.Type.Elem()); err != nil {
				return fmt.Errorf("%s (%s) map value: %w", name, tag, err)
			}
			continue
		}
		if err := validateCoercer(field.Type); err != nil {
			return fmt.Errorf("%s (%s): %w", name, tag, err)
		}
	}
	return nil
}

func validateCoercer(typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Implements(textUnmarshalerType) ||
		reflect.PointerTo(typ).Implements(textUnmarshalerType) ||
		typ.Implements(binderType) ||
		reflect.PointerTo(typ).Implements(binderType) ||
		typ == durationType ||
		typ == urlType {
		return nil
	}
	switch typ.Kind() {
	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Bool, reflect.Float32, reflect.Float64:
		return nil
	default:
		return errors.New("unsupported type: " + typ.String())
	}
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

func fieldRequired(field reflect.StructField) bool {
	required := field.Type.Kind() != reflect.Pointer
	for option := range strings.SplitSeq(field.Tag.Get("aku"), ",") {
		switch strings.TrimSpace(option) {
		case "optional":
			required = false
		case "required":
			required = true
		}
	}
	return required
}

// Compiler inspects a generic input type once at startup and builds
// a static Extractor and Schema that avoids repeated per-request type inspection.
func Compiler[T any]() (Extractor[T], *Schema) {
	var t T
	typ := reflect.TypeOf(t)
	schema := &Schema{}

	// If the input is not a struct, or is empty, we don't need to extract anything.
	if typ == nil || typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, t *T, v reflect.Value, cfg *Config) error {
			return nil
		}, schema
	}

	// Build up a list of step functions that each handle one section
	// (Path, Query, Header, Body) and execute them in order at runtime.
	var steps []internalExtractor

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// For MVP, we only care about specific exported top-level struct sections.
		switch field.Name {
		case "Path":
			ex, params := compilePath(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Parameters = append(schema.Parameters, params...)
		case "Query":
			ex, params := compileQuery(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Parameters = append(schema.Parameters, params...)
		case "Header":
			ex, params := compileHeader(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Parameters = append(schema.Parameters, params...)
		case "Form":
			ex, params := compileForm(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Parameters = append(schema.Parameters, params...)
		case "Body":
			steps = append(steps, internalExtractor(compileBody(i, field.Type)))
			schema.Body = field.Type
		case "Ctx":
			ex, params := compileCtx(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Parameters = append(schema.Parameters, params...)
		case "Auth":
			ex, auth := compileAuth(i, field.Type)
			steps = append(steps, internalExtractor(ex))
			schema.Auth = append(schema.Auth, auth...)
		}
	}

	// If the type implements interface{ Validate() error }, call it after extraction.
	var validator func(*T) error
	if _, ok := any((*T)(nil)).(interface{ Validate() error }); ok {
		validator = func(t *T) error {
			return any(t).(interface{ Validate() error }).Validate()
		}
	}

	// The returned Extractor simply runs all the compiled steps.
	return func(ctx context.Context, r *http.Request, t *T, v reflect.Value, cfg *Config) error {
		for _, step := range steps {
			if err := step(ctx, r, v, cfg); err != nil {
				return err
			}
		}
		if err := validateStrictInputs(r, schema, cfg); err != nil {
			return err
		}
		if validator != nil {
			return validator(t)
		}
		return nil
	}, schema
}

// GetCustomMessages extracts the 'msg' tag from all fields of a struct and its sub-structs.
// It maps the tag name (e.g. from query, json, etc.) to the custom error message.
func GetCustomMessages(typ reflect.Type) map[string]string {
	return getCustomMessages(typ, make(map[reflect.Type]bool))
}

func getCustomMessages(typ reflect.Type, stack map[reflect.Type]bool) map[string]string {
	if typ == nil {
		return nil
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	if stack[typ] {
		return nil
	}
	stack[typ] = true
	defer delete(stack, typ)

	msgs := make(map[string]string)
	for field := range typ.Fields() {
		if field.PkgPath != "" {
			continue
		}

		// Recurse into common Aku sections
		switch field.Name {
		case "Path", "Query", "Header", "Form", "Body", "Ctx", "Auth":
			maps.Copy(msgs, getCustomMessages(field.Type, stack))
			continue
		}

		msg := field.Tag.Get("msg")
		if msg == "" {
			continue
		}

		name := field.Name
		// Check tags in priority order
		for _, tag := range []string{"json", "query", "header", "path", "form"} {
			if t := field.Tag.Get(tag); t != "" {
				name = strings.Split(t, ",")[0]
				break
			}
		}
		msgs[name] = msg
	}
	return msgs
}
