package openapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const tfmodmakeSchemaRefKey = "x-tfmodmake-schema-ref"

// PropertyWritabilityResolver can answer whether a property is writable based on
// raw spec metadata (notably for Azure specs that illegally combine $ref siblings
// with readOnly/x-ms-mutability, which some parsers drop during ref resolution).
//
// Example (as found in some Azure specs):
//
//	"powerState": {
//	  "$ref": "#/definitions/PowerState",
//	  "readOnly": true
//	}
//
// JSON Schema/OpenAPI treat $ref as a full replacement, so parsers commonly
// discard sibling fields like readOnly. If we only look at the resolved schema
// (PowerState), we incorrectly treat the property as writable and emit it into
// the request body, which AzAPI schema validation rejects.
//
// Returning ok=false means "unknown" and callers should fall back to schema-level
// writability (e.g. Schema.ReadOnly, schema.Extensions).
type PropertyWritabilityResolver interface {
	IsWritable(containerSchemaRef string, propName string) (writable bool, ok bool)
}

type rawSpecPropertyWritabilityResolver struct {
	root any
}

func NewPropertyWritabilityResolver(specPath string) (PropertyWritabilityResolver, error) {
	data, err := readSpecBytes(specPath)
	if err != nil {
		return nil, err
	}
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &rawSpecPropertyWritabilityResolver{root: root}, nil
}

func readSpecBytes(specPath string) ([]byte, error) {
	if u, err := url.Parse(specPath); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		resp, err := http.Get(specPath) // #nosec G107 -- CLI tool loads user-provided spec URLs
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(specPath)
}

func (r *rawSpecPropertyWritabilityResolver) IsWritable(containerSchemaRef string, propName string) (bool, bool) {
	container := lookupSchemaObjectByRef(r.root, containerSchemaRef)
	if container == nil {
		return false, false
	}
	props, _ := container["properties"].(map[string]any)
	if props == nil {
		return false, false
	}
	p, _ := props[propName].(map[string]any)
	if p == nil {
		return false, false
	}

	if ro, ok := p["readOnly"].(bool); ok && ro {
		return false, true
	}

	if raw, ok := p["x-ms-mutability"]; ok {
		mutabilities := parseStringArray(raw)
		if len(mutabilities) > 0 {
			for _, m := range mutabilities {
				m = strings.ToLower(strings.TrimSpace(m))
				if m == "create" || m == "update" {
					return true, true
				}
			}
			return false, true
		}
	}

	return false, false
}

func parseStringArray(raw any) []string {
	var out []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, v...)
	case json.RawMessage:
		var decoded []string
		if err := json.Unmarshal(v, &decoded); err == nil {
			out = append(out, decoded...)
		}
	}
	return out
}

func lookupSchemaObjectByRef(root any, ref string) map[string]any {
	if ref == "" {
		return nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}

	m, _ := root.(map[string]any)
	if m == nil {
		return nil
	}

	if strings.HasPrefix(ref, "#/definitions/") {
		name := strings.TrimPrefix(ref, "#/definitions/")
		defs, _ := m["definitions"].(map[string]any)
		if defs == nil {
			return nil
		}
		def, _ := defs[name].(map[string]any)
		return def
	}

	if strings.HasPrefix(ref, "#/components/schemas/") {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		comps, _ := m["components"].(map[string]any)
		if comps == nil {
			return nil
		}
		schemas, _ := comps["schemas"].(map[string]any)
		if schemas == nil {
			return nil
		}
		s, _ := schemas[name].(map[string]any)
		return s
	}

	return nil
}

// AnnotateSchemaRefOrigins propagates schema ref strings (SchemaRef.Ref) onto
// the referenced Schema.Value via Extensions so downstream code can understand
// which definition/component a schema came from.
func AnnotateSchemaRefOrigins(schema *openapi3.Schema) {
	annotateSchemaRefOrigins(schema, make(map[*openapi3.Schema]struct{}))
}

func annotateSchemaRefOrigins(schema *openapi3.Schema, visited map[*openapi3.Schema]struct{}) {
	if schema == nil {
		return
	}
	if _, seen := visited[schema]; seen {
		return
	}
	visited[schema] = struct{}{}

	for _, ref := range schema.AllOf {
		if ref == nil || ref.Value == nil {
			continue
		}
		if ref.Ref != "" {
			ensureSchemaRefExtension(ref.Value, ref.Ref)
		}
		annotateSchemaRefOrigins(ref.Value, visited)
	}

	for _, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		if propRef.Ref != "" {
			ensureSchemaRefExtension(propRef.Value, propRef.Ref)
		}
		annotateSchemaRefOrigins(propRef.Value, visited)
	}

	if schema.Items != nil && schema.Items.Value != nil {
		if schema.Items.Ref != "" {
			ensureSchemaRefExtension(schema.Items.Value, schema.Items.Ref)
		}
		annotateSchemaRefOrigins(schema.Items.Value, visited)
	}

	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		if schema.AdditionalProperties.Schema.Ref != "" {
			ensureSchemaRefExtension(schema.AdditionalProperties.Schema.Value, schema.AdditionalProperties.Schema.Ref)
		}
		annotateSchemaRefOrigins(schema.AdditionalProperties.Schema.Value, visited)
	}
}

func ensureSchemaRefExtension(schema *openapi3.Schema, ref string) {
	if schema == nil || ref == "" {
		return
	}
	if schema.Extensions == nil {
		schema.Extensions = make(map[string]any)
	}
	if _, ok := schema.Extensions[tfmodmakeSchemaRefKey]; !ok {
		schema.Extensions[tfmodmakeSchemaRefKey] = ref
	}
}

// ApplyPropertyWritabilityOverrides uses the provided resolver to mark properties
// as readOnly on a per-property basis, even when they are $ref properties where
// the parser dropped sibling metadata.
//
// This mutates the schema graph rooted at schema.
func ApplyPropertyWritabilityOverrides(schema *openapi3.Schema, resolver PropertyWritabilityResolver) {
	applyPropertyWritabilityOverrides(schema, resolver, make(map[*openapi3.Schema]struct{}))
}

func applyPropertyWritabilityOverrides(schema *openapi3.Schema, resolver PropertyWritabilityResolver, visited map[*openapi3.Schema]struct{}) {
	if schema == nil || resolver == nil {
		return
	}
	if _, seen := visited[schema]; seen {
		return
	}
	visited[schema] = struct{}{}

	containerRef, _ := schema.Extensions[tfmodmakeSchemaRefKey].(string)

	for propName, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}

		if containerRef != "" {
			writable, ok := resolver.IsWritable(containerRef, propName)
			if ok && !writable {
				clone := *propRef.Value
				clone.ReadOnly = true
				propRef.Value = &clone
			}
		}

		applyPropertyWritabilityOverrides(propRef.Value, resolver, visited)
	}

	for _, ref := range schema.AllOf {
		if ref == nil || ref.Value == nil {
			continue
		}
		applyPropertyWritabilityOverrides(ref.Value, resolver, visited)
	}

	if schema.Items != nil && schema.Items.Value != nil {
		applyPropertyWritabilityOverrides(schema.Items.Value, resolver, visited)
	}

	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		applyPropertyWritabilityOverrides(schema.AdditionalProperties.Schema.Value, resolver, visited)
	}
}
