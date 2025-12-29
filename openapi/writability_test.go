package openapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPropertyWritabilityOverrides_RefSiblingReadOnly(t *testing.T) {
	t.Parallel()

	spec := `{
  "swagger": "2.0",
  "info": {"title": "test", "version": "2025-01-01"},
  "paths": {
    "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Test/widgets/{widgetName}": {
      "put": {
        "parameters": [
          {
            "name": "parameters",
            "in": "body",
            "required": true,
            "schema": {"$ref": "#/definitions/Widget"}
          }
        ],
        "responses": {"200": {"description": "ok"}}
      }
    }
  },
  "definitions": {
    "Widget": {
      "type": "object",
      "properties": {
        "properties": {"$ref": "#/definitions/WidgetProperties"}
      }
    },
    "WidgetProperties": {
      "type": "object",
      "properties": {
        "powerState": {"$ref": "#/definitions/PowerState", "readOnly": true},
        "name": {"type": "string"}
      }
    },
    "PowerState": {
      "type": "object",
      "properties": {
        "code": {"type": "string"}
      }
    }
  }
}`

	tmp := t.TempDir()
	path := filepath.Join(tmp, "spec.json")
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatalf("write temp spec: %v", err)
	}

	doc, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}

	schema, err := FindResource(doc, "Microsoft.Test/widgets")
	if err != nil {
		t.Fatalf("FindResource: %v", err)
	}

	propsRef := schema.Properties["properties"]
	if propsRef == nil || propsRef.Value == nil {
		t.Fatalf("expected root.properties schema to exist")
	}

	// The parser typically drops readOnly when it appears alongside $ref.
	before := propsRef.Value.Properties["powerState"]
	if before == nil || before.Value == nil {
		t.Fatalf("expected properties.powerState schema to exist")
	}
	if before.Value.ReadOnly {
		t.Fatalf("expected powerState ReadOnly to be false before override")
	}

	AnnotateSchemaRefOrigins(schema)
	resolver, err := NewPropertyWritabilityResolver(path)
	if err != nil {
		t.Fatalf("NewPropertyWritabilityResolver: %v", err)
	}
	ApplyPropertyWritabilityOverrides(schema, resolver)

	after := propsRef.Value.Properties["powerState"]
	if after == nil || after.Value == nil {
		t.Fatalf("expected properties.powerState schema to exist after override")
	}
	if !after.Value.ReadOnly {
		t.Fatalf("expected powerState ReadOnly to be true after override")
	}
}
