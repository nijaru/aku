package bind

import (
	"encoding"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"time"
)

// Binder is the interface that can be implemented by types to customize
// how they are extracted from path, query, or header parameters.
type Binder interface {
	UnmarshalAku(val string) error
}

var (
	binderType          = reflect.TypeFor[Binder]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	durationType        = reflect.TypeFor[time.Duration]()
	urlType             = reflect.TypeFor[url.URL]()
)

// Coercer is a function that converts a string value into a target reflect.Value.
type Coercer func(string, reflect.Value) error

// PrecompileCoercer creates a specialized coercer for the given type once at startup.
func PrecompileCoercer(typ reflect.Type) Coercer {
	// Pointers
	if typ.Kind() == reflect.Pointer {
		elemTyp := typ.Elem()
		subCoercer := PrecompileCoercer(elemTyp)
		return func(s string, v reflect.Value) error {
			if v.IsNil() {
				v.Set(reflect.New(elemTyp))
			}
			return subCoercer(s, v.Elem())
		}
	}

	// TextUnmarshaler
	if typ.Implements(textUnmarshalerType) {
		return func(s string, v reflect.Value) error {
			return v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
	}
	if reflect.PointerTo(typ).Implements(textUnmarshalerType) {
		return func(s string, v reflect.Value) error {
			if !v.CanAddr() {
				return errors.New("cannot address value to call UnmarshalText")
			}
			return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
	}

	// Binder
	if typ.Implements(binderType) {
		return func(s string, v reflect.Value) error {
			return v.Interface().(Binder).UnmarshalAku(s)
		}
	}
	if reflect.PointerTo(typ).Implements(binderType) {
		return func(s string, v reflect.Value) error {
			if !v.CanAddr() {
				return errors.New("cannot address value to call UnmarshalAku")
			}
			return v.Addr().Interface().(Binder).UnmarshalAku(s)
		}
	}

	// Specialized types
	if typ == durationType {
		return func(s string, v reflect.Value) error {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("invalid duration: %w", err)
			}
			v.SetInt(int64(d))
			return nil
		}
	}
	if typ == urlType {
		return func(s string, v reflect.Value) error {
			u, err := url.Parse(s)
			if err != nil {
				return fmt.Errorf("invalid url: %w", err)
			}
			v.Set(reflect.ValueOf(*u))
			return nil
		}
	}

	switch typ.Kind() {
	case reflect.String:
		return func(s string, v reflect.Value) error {
			v.SetString(s)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(s string, v reflect.Value) error {
			i, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid integer: %w", err)
			}
			if v.OverflowInt(i) {
				return errors.New("integer overflow")
			}
			v.SetInt(i)
			return nil
		}
	case reflect.Bool:
		return func(s string, v reflect.Value) error {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return fmt.Errorf("invalid boolean: %w", err)
			}
			v.SetBool(b)
			return nil
		}
	case reflect.Float32, reflect.Float64:
		return func(s string, v reflect.Value) error {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return fmt.Errorf("invalid float: %w", err)
			}
			if v.OverflowFloat(f) {
				return errors.New("float overflow")
			}
			v.SetFloat(f)
			return nil
		}
	}

	return func(s string, v reflect.Value) error {
		return fmt.Errorf("unsupported type: %s", typ.String())
	}
}

// coerce converts a string value into the target reflect.Value's type.
// Keep for backward compatibility or simple cases, but PrecompileCoercer is preferred.
func coerce(s string, v reflect.Value) error {
	return PrecompileCoercer(v.Type())(s, v)
}
