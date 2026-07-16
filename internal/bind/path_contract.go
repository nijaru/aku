package bind

import (
	"fmt"
	"strings"
)

// ValidatePathPattern makes the route pattern and Path bindings agree before
// the route is installed in net/http.ServeMux.
func ValidatePathPattern(pattern string, schema *Schema) error {
	patternNames := make(map[string]struct{})
	for rest := pattern; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			break
		}
		rest = rest[start+1:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			break
		}
		name := rest[:end]
		rest = rest[end+1:]
		if name == "$" {
			continue
		}
		name = strings.TrimSuffix(name, "...")
		if name != "" {
			patternNames[name] = struct{}{}
		}
	}

	boundNames := make(map[string]struct{})
	if schema != nil {
		for _, parameter := range schema.Parameters {
			if parameter.In == "path" {
				boundNames[parameter.Name] = struct{}{}
			}
		}
	}

	for name := range patternNames {
		if _, ok := boundNames[name]; !ok {
			return fmt.Errorf("path pattern %q has no matching path binding for %q", pattern, name)
		}
	}
	for name := range boundNames {
		if _, ok := patternNames[name]; !ok {
			return fmt.Errorf("path binding %q is missing from path pattern %q", name, pattern)
		}
	}
	return nil
}
