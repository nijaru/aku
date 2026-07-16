package bind

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

func validateAuthFields(typ reflect.Type) error {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}

		tag := field.Tag.Get("auth")
		base := field.Type
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		isBearer := strings.EqualFold(base.Name(), "Bearer") || tag == "bearer"
		if isBearer {
			if !validAuthDestination(field.Type, "Token") {
				return fmt.Errorf(
					"auth.%s: bearer credentials cannot be assigned to %s",
					field.Name,
					field.Type,
				)
			}
			continue
		}

		if strings.HasPrefix(tag, "apikey:") {
			parts := strings.SplitN(tag, ":", 3)
			if len(parts) != 3 || (parts[1] != "header" && parts[1] != "query") || parts[2] == "" {
				return fmt.Errorf("auth.%s: invalid API key declaration %q", field.Name, tag)
			}
			if !validAuthDestination(field.Type, "Key") {
				return fmt.Errorf(
					"auth.%s: API key cannot be assigned to %s",
					field.Name,
					field.Type,
				)
			}
			continue
		}

		if strings.Contains(tag, ":") {
			return fmt.Errorf("auth.%s: unsupported authentication declaration %q", field.Name, tag)
		}

		if tag != "" && base.Kind() != reflect.String {
			return fmt.Errorf("auth.%s: unsupported authentication declaration %q", field.Name, tag)
		}
	}
	return nil
}

func validAuthDestination(typ reflect.Type, fieldName string) bool {
	base := typ
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() == reflect.String {
		return true
	}
	if base.Kind() != reflect.Struct {
		return false
	}
	field, ok := base.FieldByName(fieldName)
	return ok && field.PkgPath == "" && field.Type.Kind() == reflect.String
}

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
		authType string // "bearer" | "apikey"
		paramKey string // the tag value (e.g. scheme name or header/query param name)
		location string // "header" | "query" (for apikey)
		required bool
	}

	var steps []authStep
	var schemes []AuthScheme

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}

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
			required := fieldRequired(field)
			steps = append(steps, authStep{
				fieldIdx: i,
				authType: "bearer",
				paramKey: name,
				required: required,
			})
			schemes = append(schemes, AuthScheme{
				Name:        name,
				Type:        "http",
				Scheme:      "bearer",
				BearerFmt:   "JWT",
				Required:    required,
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
			if (location != "header" && location != "query") || paramKey == "" {
				continue
			}
			required := fieldRequired(field)

			name := field.Name
			if name == "" {
				name = paramKey
			}

			steps = append(steps, authStep{
				fieldIdx: i,
				authType: "apikey",
				paramKey: paramKey,
				location: location,
				required: required,
			})
			schemes = append(schemes, AuthScheme{
				Name:        name,
				Type:        "apiKey",
				In:          location,
				ParamName:   paramKey,
				Required:    required,
				Description: fmt.Sprintf("API key authentication via %s %s", location, paramKey),
			})
			continue
		}

		// Fallback: if the type has String() method or is a string, treat as apikey:header
		if fTyp.Kind() == reflect.String && authTag != "" {
			required := fieldRequired(field)
			steps = append(steps, authStep{
				fieldIdx: i,
				authType: "apikey",
				paramKey: authTag,
				location: "header",
				required: required,
			})
			schemes = append(schemes, AuthScheme{
				Name:        field.Name,
				Type:        "apiKey",
				In:          "header",
				ParamName:   authTag,
				Required:    required,
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
							Field:     step.paramKey,
							Source:    "auth",
							Err:       errors.New("missing bearer token"),
							Challenge: "Bearer",
						}
					}
					continue
				}
				parts := strings.Fields(auth)
				if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
					if step.required {
						return &BindError{
							Field:     step.paramKey,
							Source:    "auth",
							Err:       errors.New("invalid authorization scheme, expected Bearer"),
							Challenge: "Bearer",
						}
					}
					continue
				}
				token := parts[1]
				if token == "" {
					if step.required {
						return &BindError{
							Field:     step.paramKey,
							Source:    "auth",
							Err:       errors.New("empty bearer token"),
							Challenge: "Bearer",
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
					if tokenField.IsValid() && tokenField.CanSet() &&
						tokenField.Kind() == reflect.String {
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
					val, _ = rawQueryLookup(r.URL.RawQuery, step.paramKey)
				}

				if val == "" {
					if step.required {
						return &BindError{
							Field:  step.paramKey,
							Source: "auth",
							Err:    errors.New("missing API key"),
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
					if tokenField.IsValid() && tokenField.CanSet() && tokenField.Kind() == reflect.String {
						tokenField.SetString(val)
					}
				}
			}
		}

		return nil
	}

	return extractor, schemes
}
