package bind

import (
	"context"
	"net/http"
	"reflect"
)

// ContextKey is a custom type used for context value lookups to avoid collisions
// with built-in string types, satisfying standard go linters (e.g., SA1029).
type ContextKey string

func compileCtx(sectionIdx int, typ reflect.Type) (func(context.Context, *http.Request, reflect.Value, *Config) error, []Parameter) {
	if typ.Kind() != reflect.Struct {
		return nil, nil
	}

	var extractors []func(context.Context, reflect.Value, *Config) error
	var params []Parameter

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("ctx")
		if tag == "" {
			continue
		}

		params = append(params, Parameter{
			Name:     field.Name,
			In:       "context",
			Type:     field.Type,
			Required: true,
		})

		idx := i
		extractors = append(extractors, func(ctx context.Context, v reflect.Value, cfg *Config) error {
			section := v.Field(sectionIdx)
			sectionVal := section
			if section.CanAddr() {
				sectionVal = section.Addr()
			} else {
				return nil
			}

			// Look up using the custom ContextKey type
			val := ctx.Value(ContextKey(tag))
			if val == nil {
				// Fallback to string key for backwards compatibility
				val = ctx.Value(tag)
				if val == nil {
					return nil
				}
			}

			fieldVal := sectionVal.Elem().Field(idx)
			if !fieldVal.CanSet() {
				return nil
			}

			reflectVal := reflect.ValueOf(val)
			targetType := field.Type

			if reflectVal.Type().AssignableTo(targetType) {
				fieldVal.Set(reflectVal)
			}

			return nil
		})
	}

	if len(extractors) == 0 {
		return nil, nil
	}

	extractor := func(ctx context.Context, _ *http.Request, v reflect.Value, cfg *Config) error {
		for _, ex := range extractors {
			if err := ex(ctx, v, cfg); err != nil {
				return err
			}
		}
		return nil
	}

	return extractor, params
}

func compileCtxFields(sectionIdx int, typ reflect.Type) (func(context.Context, *http.Request, reflect.Value, *Config) error, []Parameter) {
	return compileCtx(sectionIdx, typ)
}
