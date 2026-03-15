package bind

import (
	"context"
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

		// Support recursion for structs that are not Custom Binders and do not implement TextUnmarshaler
		isBinder := fTyp.Implements(binderType) || reflect.PointerTo(fTyp).Implements(binderType)
		isText := fTyp.Implements(textUnmarshalerType) || reflect.PointerTo(fTyp).Implements(textUnmarshalerType)

		if fTyp.Kind() == reflect.Struct && fTyp != reflect.TypeOf(time.Time{}) && !isBinder && !isText {
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

		if isSlice {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			sliceTyp := field.Type
			steps = append(steps, func(query url.Values, v reflect.Value) error {
				vals := query[fieldName]
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
				return nil
			})
		} else if isMap {
			elemCoercer := PrecompileCoercer(field.Type.Elem())
			mapTyp := field.Type
			steps = append(steps, func(query url.Values, v reflect.Value) error {
				// Support name[key]=val pattern for maps
				prefix := fieldName + "["
				m := reflect.MakeMap(mapTyp)
				found := false
				for k, vals := range query {
					if len(k) > len(prefix)+1 && k[:len(prefix)] == prefix && k[len(k)-1] == ']' {
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
				}
				return nil
			})
		} else {
			coercer := PrecompileCoercer(field.Type)
			steps = append(steps, func(query url.Values, v reflect.Value) error {
				val := query.Get(fieldName)
				if val != "" {
					if err := coercer(val, v.Field(fieldIdx)); err != nil {
						return &BindError{Field: fieldName, Source: "query", Err: err}
					}
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
		})
	}

	return steps, params
}
