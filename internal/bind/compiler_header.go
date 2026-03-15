package bind

import (
	"context"
	"net/http"
	"reflect"
	"time"
)

// compileHeader creates an Extractor for the Header section of the request struct.
func compileHeader(sectionIdx int, typ reflect.Type) (Extractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	steps, params := compileHeaderLevel(typ, "")

	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
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

		// Support recursion for structs that are not Custom Binders and do not implement TextUnmarshaler
		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) || reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeOf(time.Time{}) && !isBinder && !isText {
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
