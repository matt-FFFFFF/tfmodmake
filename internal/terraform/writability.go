package terraform

import (
	"encoding/json"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func isWritableProperty(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.ReadOnly {
		return false
	}

	// Azure specs often annotate mutability using x-ms-mutability.
	// If it's present and does not include create/update, treat it as non-writable.
	if schema.Extensions != nil {
		if raw, ok := schema.Extensions["x-ms-mutability"]; ok {
			mutabilities := make([]string, 0)
			switch v := raw.(type) {
			case json.RawMessage:
				var decoded []string
				if err := json.Unmarshal(v, &decoded); err == nil {
					for _, item := range decoded {
						item = strings.ToLower(strings.TrimSpace(item))
						if item != "" {
							mutabilities = append(mutabilities, item)
						}
					}
				}
			case []string:
				for _, item := range v {
					item = strings.ToLower(strings.TrimSpace(item))
					if item != "" {
						mutabilities = append(mutabilities, item)
					}
				}
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						mutabilities = append(mutabilities, strings.ToLower(strings.TrimSpace(s)))
					}
				}
			}

			if len(mutabilities) > 0 {
				for _, m := range mutabilities {
					if m == "create" || m == "update" {
						return true
					}
				}
				return false
			}
		}
	}

	return true
}

func hasWritableProperty(schema *openapi3.Schema, path string) bool {
	if schema == nil || path == "" {
		return false
	}
	segments := strings.Split(path, ".")
	return hasWritablePropertyRecursive(schema, segments, make(map[*openapi3.Schema]struct{}))
}

func hasWritablePropertyRecursive(schema *openapi3.Schema, segments []string, visited map[*openapi3.Schema]struct{}) bool {
	if schema == nil || len(segments) == 0 {
		return false
	}
	if _, seen := visited[schema]; seen {
		return false
	}
	visited[schema] = struct{}{}

	propName := segments[0]
	propRef, ok := schema.Properties[propName]
	if ok && propRef != nil && propRef.Value != nil && isWritableProperty(propRef.Value) {
		if len(segments) == 1 {
			return true
		}
		if hasWritablePropertyRecursive(propRef.Value, segments[1:], visited) {
			return true
		}
	}

	for _, ref := range schema.AllOf {
		if ref == nil || ref.Value == nil {
			continue
		}
		if hasWritablePropertyRecursive(ref.Value, segments, visited) {
			return true
		}
	}

	return false
}
