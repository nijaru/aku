package bind

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
)

// compileHeader creates an internalExtractor for the Header section of the request struct.
func compileHeader(sectionIdx int, typ reflect.Type) (internalExtractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	steps, params := compileHeaderLevel(typ, "")

	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		section := v.Field(sectionIdx)

		for _, step := range steps {
			if err := step(r.Header, section, nil); err != nil {
				return err
			}
		}

		return nil
	}, params
}

func headerValues(header http.Header, name string) []string {
	if values := header.Values(name); len(values) > 0 {
		return values
	}
	for key, values := range header {
		if strings.EqualFold(key, name) {
			return values
		}
	}
	return nil
}

func isStandardHeader(h string) bool {
	h = strings.ToLower(h)
	// Standard request and proxy headers are transport concerns rather than
	// application inputs. Strict mode should not reject them merely because a
	// route does not bind them explicitly.
	standard := []string{
		"accept", "accept-charset", "accept-encoding", "accept-language",
		"accept-patch", "accept-post", "accept-ranges", "age", "cache-control",
		"authorization", "connection", "content-encoding", "content-language", "cookie",
		"content-length",
		"content-location", "content-md5", "content-range", "content-type",
		"date", "expect", "forwarded", "host", "if-match", "if-modified-since",
		"if-none-match", "if-range", "if-unmodified-since", "keep-alive",
		"last-modified", "max-forwards", "origin", "pragma", "proxy-authenticate",
		"proxy-authorization", "range", "referer", "retry-after", "server",
		"te", "trailer", "transfer-encoding", "traceparent", "tracestate",
		"upgrade", "upgrade-insecure-requests", "user-agent", "vary", "via",
		"warning", "www-authenticate", "x-forwarded-for", "x-forwarded-host",
		"x-forwarded-proto", "x-request-id",
	}
	return slices.Contains(standard, h) || strings.HasPrefix(h, "sec-")
}

type headerStep func(http.Header, reflect.Value, map[string]struct{}) error

func compileHeaderLevel(typ reflect.Type, prefix string) ([]headerStep, []Parameter) {
	var steps []headerStep
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
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

		// Support recursion for structs that are not Custom Binders and do not implement TextUnmarshaler
		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) ||
			reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeFor[time.Time]() && !isBinder &&
			!isText {
			subSteps, subParams := compileHeaderLevel(fTyp, name)
			subPrefix := name + "["
			fieldIdx := i
			isRequired := fieldRequired(field)
			steps = append(
				steps,
				func(h http.Header, v reflect.Value, consumed map[string]struct{}) error {
					// Only allocate/recurse if there's actually data for this struct
					found := false
					for k := range h {
						if len(k) > len(subPrefix) &&
							strings.EqualFold(k[:len(subPrefix)], subPrefix) {
							found = true
							if consumed != nil {
								// Sub-steps will mark individual keys
							} else {
								break
							}
						}
					}
					if !found {
						if isRequired {
							return &BindError{
								Field:  name,
								Source: "header",
								Err:    errors.New("is required"),
							}
						}
						return nil
					}

					f := v.Field(fieldIdx)
					if f.Kind() == reflect.Pointer {
						if f.IsNil() {
							f.Set(reflect.New(f.Type().Elem()))
						}
						f = f.Elem()
					}
					for _, subStep := range subSteps {
						if err := subStep(h, f, consumed); err != nil {
							return err
						}
					}
					return nil
				},
			)
			params = append(params, subParams...)
			continue
		}

		isSlice := field.Type.Kind() == reflect.Slice
		isMap := field.Type.Kind() == reflect.Map
		fieldIdx := i
		fieldName := name
		isRequired := fieldRequired(field)

		if isSlice {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			sliceTyp := field.Type
			steps = append(
				steps,
				func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
					vals := headerValues(header, fieldName)
					if len(vals) > 0 {
						if consumed != nil {
							consumed[http.CanonicalHeaderKey(fieldName)] = struct{}{}
						}
						if len(vals) > 0 {
							f := v.Field(fieldIdx)
							slice := reflect.MakeSlice(sliceTyp, len(vals), len(vals))
							for i, val := range vals {
								if err := elemCoercer(val, slice.Index(i)); err != nil {
									return &BindError{Field: fieldName, Source: "header", Err: err}
								}
							}
							f.Set(slice)
						}
					} else if isRequired {
						return &BindError{Field: fieldName, Source: "header", Err: errors.New("is required")}
					}
					return nil
				},
			)
		} else if isMap {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			keyCoercer := PrecompileCoercer(field.Type.Key())
			mapTyp := field.Type
			isPrefix := len(fieldName) > 0 && fieldName[len(fieldName)-1] == '-'
			steps = append(steps, func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
				m := reflect.MakeMap(mapTyp)
				found := false
				for k, vals := range header {
					if isPrefix {
						if len(k) > len(fieldName) && strings.EqualFold(k[:len(fieldName)], fieldName) {
							if consumed != nil {
								consumed[http.CanonicalHeaderKey(k)] = struct{}{}
							}
							if len(vals) == 0 {
								continue
							}
							key := k[len(fieldName):]
							val := vals[0]
							keyVal := reflect.New(mapTyp.Key()).Elem()
							if err := keyCoercer(key, keyVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							valVal := reflect.New(mapTyp.Elem()).Elem()
							if err := elemCoercer(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(keyVal, valVal)
							found = true
						}
					} else {
						mapPrefix := fieldName + "["
						if len(k) > len(mapPrefix)+1 && strings.EqualFold(k[:len(mapPrefix)], mapPrefix) && k[len(k)-1] == ']' {
							if consumed != nil {
								consumed[http.CanonicalHeaderKey(k)] = struct{}{}
							}
							if len(vals) == 0 {
								continue
							}
							key := k[len(mapPrefix) : len(k)-1]
							val := vals[0]
							keyVal := reflect.New(mapTyp.Key()).Elem()
							if err := keyCoercer(key, keyVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							valVal := reflect.New(mapTyp.Elem()).Elem()
							if err := elemCoercer(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(keyVal, valVal)
							found = true
						}
					}
				}
				if found {
					v.Field(fieldIdx).Set(m)
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "header", Err: errors.New("is required")}
				}
				return nil
			})
		} else {
			coercer := PrecompileCoercer(field.Type)
			steps = append(steps, func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
				vals := headerValues(header, fieldName)
				if len(vals) > 0 {
					if consumed != nil {
						consumed[http.CanonicalHeaderKey(fieldName)] = struct{}{}
					}
					val := vals[0]
					if val != "" {
						if err := coercer(val, v.Field(fieldIdx)); err != nil {
							return &BindError{Field: fieldName, Source: "header", Err: err}
						}
					} else if isRequired {
						return &BindError{Field: fieldName, Source: "header", Err: errors.New("is required")}
					}
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "header", Err: errors.New("is required")}
				}
				return nil
			})
		}

		params = append(params, Parameter{
			Name:     name,
			In:       "header",
			Type:     field.Type,
			Required: fieldRequired(field),
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return steps, params
}
