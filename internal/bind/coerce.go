package bind

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// coerce converts a string value into the target reflect.Value's type.
// Supported types: string, int, int8, int16, int32, int64, bool, float32, float64, time.Time, and pointers to these types.
func coerce(s string, v reflect.Value) error {
	if v.Type() == reflect.TypeOf(time.Time{}) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("invalid time format (RFC3339): %w", err)
		}
		v.Set(reflect.ValueOf(t))
		return nil
	}

	if v.Kind() == reflect.Pointer {
		// Create a new value of the underlying type and coerce the string into it.
		elemTyp := v.Type().Elem()
		newVal := reflect.New(elemTyp).Elem()
		if err := coerce(s, newVal); err != nil {
			return err
		}
		// Set the pointer to point to the newly created value.
		v.Set(newVal.Addr())
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
