package bind

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"time"
)

// compileQuery creates an internalExtractor for the Query section of the request struct.
func compileQuery(sectionIdx int, typ reflect.Type) (internalExtractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
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

		if cfg.StrictQuery {
			for k := range query {
				if _, ok := consumed[k]; !ok {
					return &BindError{
						Field:  k,
						Source: "query",
						Err:    fmt.Errorf("unknown parameter"),
					}
				}
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
		isRequired := field.Type.Kind() != reflect.Pointer

		if isSlice {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			sliceTyp := field.Type
			steps = append(
				steps,
				func(query url.Values, v reflect.Value, consumed map[string]struct{}) error {
					vals, ok := query[fieldName]
					if ok {
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
						return &BindError{Field: fieldName, Source: "query", Err: fmt.Errorf("is required")}
					}
					return nil
				},
			)
		} else if isMap {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
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
						val := vals[0] // take first for map
						valVal := reflect.New(mapTyp.Elem()).Elem()
						if err := elemCoercer(val, valVal); err != nil {
							return &BindError{Field: fieldName + "[" + key + "]", Source: "query", Err: err}
						}
						m.SetMapIndex(reflect.ValueOf(key), valVal)
						found = true
					}
				}
				if found {
					v.Field(fieldIdx).Set(m)
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "query", Err: fmt.Errorf("is required")}
				}
				return nil
			})
		} else {
			coercer := PrecompileCoercer(field.Type)
			steps = append(steps, func(query url.Values, v reflect.Value, consumed map[string]struct{}) error {
				vals, ok := query[fieldName]
				if ok {
					if consumed != nil {
						consumed[fieldName] = struct{}{}
					}
					val := vals[0]
					if val != "" {
						if err := coercer(val, v.Field(fieldIdx)); err != nil {
							return &BindError{Field: fieldName, Source: "query", Err: err}
						}
					}
				} else if isRequired {
					return &BindError{Field: fieldName, Source: "query", Err: fmt.Errorf("is required")}
				}
				return nil
			})
		}

		params = append(params, Parameter{
			Name:     name,
			In:       "query",
			Type:     field.Type,
			Required: field.Type.Kind() != reflect.Pointer,
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return steps, params
}
