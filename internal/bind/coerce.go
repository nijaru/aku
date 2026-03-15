package bind

import (
	"encoding"
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
	binderType          = reflect.TypeOf((*Binder)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	durationType        = reflect.TypeOf(time.Duration(0))
	urlType             = reflect.TypeOf(url.URL{})
)

// coerce converts a string value into the target reflect.Value's type.
func coerce(s string, v reflect.Value) error {
	typ := v.Type()

	if v.Kind() != reflect.Pointer || !v.IsNil() {
		if typ.Implements(binderType) {
			return v.Interface().(Binder).UnmarshalAku(s)
		}
		if typ.Implements(textUnmarshalerType) {
			return v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
	}

	if v.CanAddr() {
		addr := v.Addr()
		if addr.Type().Implements(binderType) {
			return addr.Interface().(Binder).UnmarshalAku(s)
		}
		if addr.Type().Implements(textUnmarshalerType) {
			return addr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
	}

	if v.Kind() == reflect.Pointer {
		// Create a new value of the underlying type and coerce the string into it.
		elemTyp := typ.Elem()
		newVal := reflect.New(elemTyp).Elem()
		if err := coerce(s, newVal); err != nil {
			return err
		}
		// Set the pointer to point to the newly created value.
		v.Set(newVal.Addr())
		return nil
	}

	// Specialized types that don't implement TextUnmarshaler or need specific handling
	if typ == durationType {
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		v.Set(reflect.ValueOf(d))
		return nil
	}
	if typ == urlType {
		u, err := url.Parse(s)
		if err != nil {
			return fmt.Errorf("invalid url: %w", err)
		}
		v.Set(reflect.ValueOf(*u))
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
		if v.OverflowInt(i) {
			return fmt.Errorf("integer overflow")
		}
		v.SetInt(i)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("invalid boolean: %w", err)
		}
		v.SetBool(b)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("invalid float: %w", err)
		}
		if v.OverflowFloat(f) {
			return fmt.Errorf("float overflow")
		}
		v.SetFloat(f)
	default:
		return fmt.Errorf("unsupported type: %s", v.Type().String())
	}
	return nil
}
