package bind

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
)

func validateStrictInputs(r *http.Request, schema *Schema, cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if cfg.StrictQuery {
		for name := range r.URL.Query() {
			if queryInputAllowed(name, schema) {
				continue
			}
			return &BindError{Field: name, Source: "query", Err: errors.New("unknown parameter")}
		}
	}

	if cfg.StrictHeader {
		for name := range r.Header {
			if isStandardHeader(name) || headerInputAllowed(name, schema) {
				continue
			}
			return &BindError{Field: name, Source: "header", Err: errors.New("unknown parameter")}
		}
	}

	return nil
}

func queryInputAllowed(name string, schema *Schema) bool {
	for _, parameter := range schema.Parameters {
		if parameter.In == "query" &&
			matchesInputName(name, parameter.Name, parameter.Type, false) {
			return true
		}
	}
	for _, auth := range schema.Auth {
		if auth.Type == "apiKey" && auth.In == "query" && name == auth.ParamName {
			return true
		}
	}
	return false
}

func headerInputAllowed(name string, schema *Schema) bool {
	for _, parameter := range schema.Parameters {
		if parameter.In == "header" &&
			matchesInputName(name, parameter.Name, parameter.Type, true) {
			return true
		}
	}
	for _, auth := range schema.Auth {
		switch {
		case auth.Type == "apiKey" && auth.In == "header":
			if strings.EqualFold(name, auth.ParamName) {
				return true
			}
		case auth.Type == "http" && strings.EqualFold(auth.Scheme, "bearer"):
			if strings.EqualFold(name, "Authorization") {
				return true
			}
		}
	}
	return false
}

func matchesInputName(name, parameter string, typ reflect.Type, fold bool) bool {
	if equalInputName(name, parameter, fold) {
		return true
	}

	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Map {
		return false
	}

	prefix := parameter + "["
	if strings.HasSuffix(parameter, "-") {
		prefix = parameter
	}
	return hasInputPrefix(name, prefix, fold)
}

func equalInputName(left, right string, fold bool) bool {
	if fold {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func hasInputPrefix(value, prefix string, fold bool) bool {
	if len(value) < len(prefix) {
		return false
	}
	if fold {
		return strings.EqualFold(value[:len(prefix)], prefix)
	}
	return value[:len(prefix)] == prefix
}
