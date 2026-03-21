package bind

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
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

		var consumed map[string]struct{}
		if cfg.StrictHeader {
			consumed = make(map[string]struct{}, len(r.Header))
		}

		for _, step := range steps {
			if err := step(r.Header, section, consumed); err != nil {
				return err
			}
		}

		if cfg.StrictHeader {
			for k := range r.Header {
				// Standard headers that we should ignore
				if isStandardHeader(k) {
					continue
				}

				if _, ok := consumed[k]; !ok {
					return &BindError{Field: k, Source: "header", Err: fmt.Errorf("unknown parameter")}
				}
			}
		}

		return nil
	}, params
}

func isStandardHeader(h string) bool {
	h = strings.ToLower(h)
	// Common standard headers to ignore in strict mode
	standard := []string{
		"accept", "accept-encoding", "accept-language", "connection",
		"cookie", "content-length", "content-type", "host",
		"origin", "referer", "sec-ch-ua", "sec-ch-ua-mobile",
		"sec-ch-ua-platform", "sec-fetch-dest", "sec-fetch-mode",
		"sec-fetch-site", "sec-fetch-user", "upgrade-insecure-requests",
		"user-agent", "x-forwarded-for", "x-forwarded-host",
		"x-forwarded-proto", "x-request-id", "traceparent", "tracestate",
	}
	for _, s := range standard {
		if h == s {
			return true
		}
	}
	return false
}

type headerStep func(http.Header, reflect.Value, map[string]struct{}) error

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

		// Support recursion for structs that are not Custom Binders and do not implement TextUnmarshaler
		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) || reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeOf(time.Time{}) && !isBinder && !isText {
			subSteps, subParams := compileHeaderLevel(fTyp, name)
			subPrefix := name + "["
			steps = append(steps, func(h http.Header, v reflect.Value, consumed map[string]struct{}) error {
				// Only allocate/recurse if there's actually data for this struct
				found := false
				for k := range h {
					if len(k) > len(subPrefix) && k[:len(subPrefix)] == subPrefix {
						found = true
						if consumed != nil {
							// Sub-steps will mark individual keys
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
					if err := subStep(h, f, consumed); err != nil {
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

		if isSlice {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			sliceTyp := field.Type
			steps = append(steps, func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
				vals, ok := header[fieldName]
				if ok {
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
				}
				return nil
			})
		} else if isMap {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			mapTyp := field.Type
			isPrefix := fieldName[len(fieldName)-1] == '-'
			steps = append(steps, func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
				m := reflect.MakeMap(mapTyp)
				found := false
				for k, vals := range header {
					if isPrefix {
						if len(k) > len(fieldName) && strings.EqualFold(k[:len(fieldName)], fieldName) {
							if consumed != nil {
								consumed[k] = struct{}{}
							}
							key := k[len(fieldName):]
							val := vals[0]
							valVal := reflect.New(mapTyp.Elem()).Elem()
							if err := elemCoercer(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(reflect.ValueOf(key), valVal)
							found = true
						}
					} else {
						mapPrefix := fieldName + "["
						if len(k) > len(mapPrefix)+1 && strings.EqualFold(k[:len(mapPrefix)], mapPrefix) && k[len(k)-1] == ']' {
							if consumed != nil {
								consumed[k] = struct{}{}
							}
							key := k[len(mapPrefix) : len(k)-1]
							val := vals[0]
							valVal := reflect.New(mapTyp.Elem()).Elem()
							if err := elemCoercer(val, valVal); err != nil {
								return &BindError{Field: k, Source: "header", Err: err}
							}
							m.SetMapIndex(reflect.ValueOf(key), valVal)
							found = true
						}
					}
				}
				if found {
					v.Field(fieldIdx).Set(m)
				}
				return nil
			})
		} else {
			coercer := PrecompileCoercer(field.Type)
			steps = append(steps, func(header http.Header, v reflect.Value, consumed map[string]struct{}) error {
				vals, ok := header[fieldName]
				if ok {
					if consumed != nil {
						consumed[http.CanonicalHeaderKey(fieldName)] = struct{}{}
					}
					val := vals[0]
					if val != "" {
						if err := coercer(val, v.Field(fieldIdx)); err != nil {
							return &BindError{Field: fieldName, Source: "header", Err: err}
						}
					}
				}
				return nil
			})
		}

		params = append(params, Parameter{
			Name:     name,
			In:       "header",
			Type:     field.Type,
			Required: field.Type.Kind() != reflect.Pointer,
			Validate: field.Tag.Get("validate"),
			Message:  field.Tag.Get("msg"),
			Example:  field.Tag.Get("example"),
		})
	}

	return steps, params
}
