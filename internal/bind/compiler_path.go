package bind

import (
	"context"
	"net/http"
	"reflect"
)

// compilePath creates an internalExtractor for the Path section of the request struct.
func compilePath(sectionIdx int, typ reflect.Type) (internalExtractor, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error { return nil }, nil
	}

	type fieldInfo struct {
		idx     int
		name    string
		coercer Coercer
	}
	var infos []fieldInfo
	var params []Parameter
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("path")
		if tag != "" {
			infos = append(infos, fieldInfo{
				idx:     i,
				name:    tag,
				coercer: PrecompileCoercer(field.Type),
			})
			params = append(params, Parameter{
				Name:     tag,
				In:       "path",
				Type:     field.Type,
				Required: field.Type.Kind() != reflect.Pointer,
				Validate: field.Tag.Get("validate"),
				Message:  field.Tag.Get("msg"),
				Example:  field.Tag.Get("example"),
			})
		}
	}

	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		section := v.Field(sectionIdx)
		for _, info := range infos {
			val := r.PathValue(info.name)
			if val != "" {
				if err := info.coercer(val, section.Field(info.idx)); err != nil {
					return &BindError{Field: info.name, Source: "path", Err: err}
				}
			}
		}
		return nil
	}, params
}
