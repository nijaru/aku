package bind

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
)

// compileBody creates an internalExtractor for the Body section of the request struct.
func compileBody(sectionIdx int, typ reflect.Type) internalExtractor {
	return func(ctx context.Context, r *http.Request, v reflect.Value, cfg *Config) error {
		if r.Body == nil || r.ContentLength == 0 {
			return nil
		}
		section := v.Field(sectionIdx)
		if err := json.NewDecoder(r.Body).Decode(section.Addr().Interface()); err != nil {
			return &BindError{Source: "body", Err: err}
		}
		return nil
	}
}
