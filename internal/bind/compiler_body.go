package bind

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(section.Addr().Interface()); err != nil {
			return &BindError{Source: "body", Err: err}
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				err = errors.New("body must contain a single JSON value")
			}
			return &BindError{Source: "body", Err: err}
		}
		return nil
	}
}
