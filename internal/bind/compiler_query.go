package bind

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

type scalarQueryStep struct {
	fieldIdx int
	name     string
	coercer  Coercer
	required bool
}

func rawQueryLookup(raw, key string) (string, bool) {
	for raw != "" {
		var pair string
		if before, after, ok := strings.Cut(raw, "&"); ok {
			pair, raw = before, after
		} else {
			pair, raw = raw, ""
		}
		k, v, _ := strings.Cut(pair, "=")
		decodedKey, err := url.QueryUnescape(k)
		if err != nil || decodedKey != key {
			continue
		}
		val, _ := url.QueryUnescape(v)
		return val, true
	}
	return "", false
}

func tryCompileScalarQuery(typ reflect.Type) ([]scalarQueryStep, []Parameter) {
	var steps []scalarQueryStep
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("query")
		if tag == "" {
			continue
		}

		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map {
			return nil, nil
		}

		fTyp := field.Type
		for fTyp.Kind() == reflect.Pointer {
			fTyp = fTyp.Elem()
		}

		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) ||
			reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeFor[time.Time]() && !isBinder &&
			!isText {
			return nil, nil
		}

		steps = append(steps, scalarQueryStep{
			fieldIdx: i,
			name:     tag,
			coercer:  PrecompileCoercer(field.Type),
			required: fieldRequired(field),
		})
		params = append(params, Parameter{
			Name:     tag,
			In:       "query",
			Type:     field.Type,
			Required: fieldRequired(field),
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return steps, params
}

// compileQuery creates an internalExtractor for the Query section of the request struct.
func compileQuery(sectionIdx int, typ reflect.Type) (internalExtractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	if scalarSteps, params := tryCompileScalarQuery(typ); scalarSteps != nil {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
			section := v.Field(sectionIdx)
			raw := r.URL.RawQuery

			for _, step := range scalarSteps {
				val, ok := rawQueryLookup(raw, step.name)
				if ok && val != "" {
					if err := step.coercer(val, section.Field(step.fieldIdx)); err != nil {
						return &BindError{Field: step.name, Source: "query", Err: err}
					}
				} else if step.required {
					return &BindError{Field: step.name, Source: "query", Err: errors.New("is required")}
				}
			}

			return nil
		}, params
	}

	steps, params := compileQueryLevel(typ, "")

	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		section := v.Field(sectionIdx)
		query := r.URL.Query()

		var consumed map[string]struct{}
		if cfg.StrictQuery {
			consumed = make(map[string]struct{}, len(query))
		}

		for _, step := range steps {
			if err := step(query, section, consumed); err != nil {
				return err
			}
		}

		return nil
	}, params
}

type queryStep func(url.Values, reflect.Value, map[string]struct{}) error

func compileQueryLevel(typ reflect.Type, prefix string) ([]queryStep, []Parameter) {
	var steps []queryStep
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
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

		// Support recursion for structs that are not Custom Binders and do not implement TextUnmarshaler
		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) ||
			reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeFor[time.Time]() && !isBinder &&
			!isText {
			subSteps, subParams := compileQueryLevel(fTyp, name)
			subPrefix := name + "["
			fieldIdx := i
			isRequired := fieldRequired(field)
			steps = append(
				steps,
				func(q url.Values, v reflect.Value, consumed map[string]struct{}) error {
					// Only allocate/recurse if there's actually data for this struct
					found := false
					for k := range q {
						if len(k) > len(subPrefix) && k[:len(subPrefix)] == subPrefix {
							found = true
							if consumed != nil {
								// We don't mark individual keys here, sub-steps will do it
							} else {
								break
							}
						}
					}
					if !found {
						if isRequired {
							return &BindError{
								Field:  name,
								Source: "query",
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
						if err := subStep(q, f, consumed); err != nil {
							return err
						}
					}
					return nil
				},
			)
			params = append(params, subParams...)
			continue
		}

		// Leaf fields (or slices/maps/binders)
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
				func(query url.Values, v reflect.Value, consumed map[string]struct{}) error {
					vals, ok := query[fieldName]
					if ok && len(vals) > 0 {
						if consumed != nil {
							consumed[fieldName] = struct{}{}
						}
						if len(vals) > 0 {
							f := v.Field(fieldIdx)
							slice := reflect.MakeSlice(sliceTyp, len(vals), len(vals))
							for i, val := range vals {
								if err := elemCoercer(val, slice.Index(i)); err != nil {
									return &BindError{Field: fieldName, Source: "query", Err: err}
								}
							}
							f.Set(slice)
						}
					} else if isRequired {
						return &BindError{Field: fieldName, Source: "query", Err: errors.New("is required")}
					}
					return nil
				},
			)
		} else if isMap {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			keyCoercer := PrecompileCoercer(field.Type.Key())
			mapTyp := field.Type
			steps = append(steps, func(query url.Values, v reflect.Value, consumed map[string]struct{}) error {
				// Support name[key]=val pattern for maps
				prefix := fieldName + "["
				m := reflect.MakeMap(mapTyp)
				found := false
				for k, vals := range query {
					if len(k) > len(prefix)+1 && k[:len(prefix)] == prefix && k[len(k)-1] == ']' {
						if consumed != nil {
							consumed[k] = struct{}{}
						}
						key := k[len(prefix) : len(k)-1]
						if len(vals) == 0 {
							continue
						}
						val := vals[0] // take first for map
						keyVal := reflect.New(mapTyp.Key()).Elem()
						if err := keyCoercer(key, keyVal); err != nil {
							return &BindError{Field: fieldName + "[" + key + "]", Source: "query", Err: err}
						}
						valVal := reflect.New(mapTyp.Elem()).Elem()
						if err := elemCoercer(val, valVal); err != nil {
							return &BindError{Field: fieldName + "[" + key + "]", Source: "query", Err: err}
						}
						m.SetMapIndex(keyVal, valVal)
						found = true
					}
				}
				if found {
					v.Field(fieldIdx).Set(m)
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "query", Err: errors.New("is required")}
				}
				return nil
			})
		} else {
			coercer := PrecompileCoercer(field.Type)
			steps = append(steps, func(query url.Values, v reflect.Value, consumed map[string]struct{}) error {
				vals, ok := query[fieldName]
				if ok && len(vals) > 0 {
					if consumed != nil {
						consumed[fieldName] = struct{}{}
					}
					val := vals[0]
					if val != "" {
						if err := coercer(val, v.Field(fieldIdx)); err != nil {
							return &BindError{Field: fieldName, Source: "query", Err: err}
						}
					} else if isRequired {
						return &BindError{Field: fieldName, Source: "query", Err: errors.New("is required")}
					}
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "query", Err: errors.New("is required")}
				}
				return nil
			})
		}

		params = append(params, Parameter{
			Name:     name,
			In:       "query",
			Type:     field.Type,
			Required: fieldRequired(field),
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return steps, params
}
