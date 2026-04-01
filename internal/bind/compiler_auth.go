package bind

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// compileAuth creates an internalExtractor for the Auth section of the request struct.
// Auth fields extract authentication credentials (Bearer tokens, API keys) from
// the request. They don't appear in OpenAPI parameters — instead, they produce
// AuthScheme entries for security definitions.
func compileAuth(sectionIdx int, typ reflect.Type) (internalExtractor, []AuthScheme) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	type authStep struct {
		fieldIdx int
		fieldTyp reflect.Type
		authType string // "bearer" | "apikey"
		paramKey string // the tag value (e.g. scheme name or header/query param name)
		location string // "header" | "query" (for apikey)
		required bool
	}

	var steps []authStep
	var schemes []AuthScheme

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Bearer token — special handling by type name
		authTag := field.Tag.Get("auth")
		fTyp := field.Type

		// Detect Bearer by type name for convenience
		isBearer := strings.EqualFold(fTyp.Name(), "Bearer") ||
			(fTyp.Kind() == reflect.Pointer && strings.EqualFold(fTyp.Elem().Name(), "Bearer"))

		if isBearer || authTag == "bearer" {
			name := authTag
			if name == "" {
				name = field.Name
				if name == "" {
					name = "bearerAuth"
				}
			}
			steps = append(steps, authStep{
				fieldIdx: i,
				fieldTyp: fTyp,
				authType: "bearer",
				paramKey: name,
				required: fTyp.Kind() != reflect.Pointer,
			})
			schemes = append(schemes, AuthScheme{
				Name:        name,
				Type:        "http",
				Scheme:      "bearer",
				BearerFmt:   "JWT",
				Required:    fTyp.Kind() != reflect.Pointer,
				Description: fmt.Sprintf("Bearer token authentication (%s)", name),
			})
			continue
		}

		// API key — extracted from tag like `auth:"apikey:header:X-API-Key"` or `auth:"apikey:query:api_key"`
		if authTag != "" && strings.HasPrefix(authTag, "apikey:") {
			parts := strings.SplitN(authTag, ":", 3)
			if len(parts) != 3 {
				continue
			}
			location := parts[1] // "header" or "query"
			paramKey := parts[2] // the header or query parameter name

			name := field.Name
			if name == "" {
				name = paramKey
			}

			steps = append(steps, authStep{
				fieldIdx: i,
				fieldTyp: fTyp,
				authType: "apikey",
				paramKey: paramKey,
				location: location,
				required: fTyp.Kind() != reflect.Pointer,
			})
			schemes = append(schemes, AuthScheme{
				Name:        name,
				Type:        "apiKey",
				In:          location,
				ParamName:   paramKey,
				Required:    fTyp.Kind() != reflect.Pointer,
				Description: fmt.Sprintf("API key authentication via %s %s", location, paramKey),
			})
			continue
		}

		// Fallback: if the type has String() method or is a string, treat as apikey:header
		if fTyp.Kind() == reflect.String && authTag != "" {
			steps = append(steps, authStep{
				fieldIdx: i,
				fieldTyp: fTyp,
				authType: "apikey",
				paramKey: authTag,
				location: "header",
				required: true,
			})
			schemes = append(schemes, AuthScheme{
				Name:        field.Name,
				Type:        "apiKey",
				In:          "header",
				ParamName:   authTag,
				Required:    true,
				Description: fmt.Sprintf("API key authentication via %s header", authTag),
			})
		}
	}

	// Build the extractor
	extractor := func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		section := v.Field(sectionIdx)

		for _, step := range steps {
			f := section.Field(step.fieldIdx)

			switch step.authType {
			case "bearer":
				auth := r.Header.Get("Authorization")
				if auth == "" {
					if step.required {
						return &BindError{
							Field:  step.paramKey,
							Source: "auth",
							Err:    fmt.Errorf("missing bearer token"),
						}
					}
					continue
				}
				const prefix = "Bearer "
				if !strings.HasPrefix(auth, prefix) {
					if step.required {
						return &BindError{
							Field:  step.paramKey,
							Source: "auth",
							Err:    fmt.Errorf("invalid authorization scheme, expected Bearer"),
						}
					}
					continue
				}
				token := strings.TrimPrefix(auth, prefix)
				if token == "" {
					if step.required {
						return &BindError{
							Field:  step.paramKey,
							Source: "auth",
							Err:    fmt.Errorf("empty bearer token"),
						}
					}
					continue
				}

				// Set the bearer value
				if f.Kind() == reflect.Pointer {
					if f.IsNil() {
						f.Set(reflect.New(f.Type().Elem()))
					}
					f = f.Elem()
				}
				if f.Kind() == reflect.Struct && f.CanAddr() {
					// Try to set Token field if it's a struct with Token field
					tokenField := f.FieldByName("Token")
					if tokenField.IsValid() && tokenField.Kind() == reflect.String {
						tokenField.SetString(token)
						continue
					}
				}
				if f.Kind() == reflect.String {
					f.SetString(token)
				}

			case "apikey":
				var val string
				switch step.location {
				case "header":
					val = r.Header.Get(step.paramKey)
				case "query":
					val = r.URL.Query().Get(step.paramKey)
				}

				if val == "" {
					if step.required {
						return &BindError{
							Field:  step.paramKey,
							Source: "auth",
							Err:    fmt.Errorf("missing API key"),
						}
					}
					continue
				}

				if f.Kind() == reflect.Pointer {
					if f.IsNil() {
						f.Set(reflect.New(f.Type().Elem()))
					}
					f = f.Elem()
				}
				if f.Kind() == reflect.String {
					f.SetString(val)
				} else if f.Kind() == reflect.Struct {
					tokenField := f.FieldByName("Key")
					if tokenField.IsValid() && tokenField.Kind() == reflect.String {
						tokenField.SetString(val)
					}
				}
			}
		}

		return nil
	}

	return extractor, schemes
}
