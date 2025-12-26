package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatChildrenAsMarkdown(t *testing.T) {
	t.Run("formats deployable and filtered out resources", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable: []ChildResource{
				{
					ResourceType: "Microsoft.App/managedEnvironments/certificates",
					Operations:   []string{"PUT", "GET", "DELETE"},
					APIVersion:   "2024-01-01",
					ExamplePaths: []string{"/subscriptions/{id}/resourceGroups/{rg}/providers/Microsoft.App/managedEnvironments/{env}/certificates/{cert}"},
					IsDeployable: true,
				},
			},
			FilteredOut: []ChildResource{
				{
					ResourceType:        "Microsoft.App/managedEnvironments/status",
					Operations:          []string{"GET"},
					APIVersion:          "2024-01-01",
					DeployabilityReason: "GET-only resource",
				},
			},
		}

		markdown := FormatChildrenAsMarkdown(result)

		assert.Contains(t, markdown, "# Deployable Child Resources")
		assert.Contains(t, markdown, "Microsoft.App/managedEnvironments/certificates")
		assert.Contains(t, markdown, "PUT, GET, DELETE")
		assert.Contains(t, markdown, "# Filtered Out")
		assert.Contains(t, markdown, "Microsoft.App/managedEnvironments/status")
		assert.Contains(t, markdown, "GET-only resource")
	})

	t.Run("handles empty results", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable:  []ChildResource{},
			FilteredOut: []ChildResource{},
		}

		markdown := FormatChildrenAsMarkdown(result)

		assert.Contains(t, markdown, "No deployable child resources found")
		assert.Contains(t, markdown, "No resources were filtered out")
	})

	t.Run("handles nil result", func(t *testing.T) {
		markdown := FormatChildrenAsMarkdown(nil)
		assert.Equal(t, "No results\n", markdown)
	})

	t.Run("sorts by resource type", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable: []ChildResource{
				{ResourceType: "Microsoft.App/managedEnvironments/storages", Operations: []string{"PUT"}, APIVersion: "2024-01-01", IsDeployable: true},
				{ResourceType: "Microsoft.App/managedEnvironments/certificates", Operations: []string{"PUT"}, APIVersion: "2024-01-01", IsDeployable: true},
			},
			FilteredOut: []ChildResource{},
		}

		markdown := FormatChildrenAsMarkdown(result)

		// Find positions of the resource types in the markdown
		certPos := strings.Index(markdown, "certificates")
		storagePos := strings.Index(markdown, "storages")

		assert.Less(t, certPos, storagePos, "certificates should appear before storages in sorted output")
	})
}

func TestFormatChildrenAsJSON(t *testing.T) {
	t.Run("formats valid JSON", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable: []ChildResource{
				{
					ResourceType: "Microsoft.App/managedEnvironments/certificates",
					Operations:   []string{"PUT", "GET"},
					APIVersion:   "2024-01-01",
					ExamplePaths: []string{"/example/path"},
					IsDeployable: true,
				},
			},
			FilteredOut: []ChildResource{
				{
					ResourceType:        "Microsoft.App/managedEnvironments/status",
					Operations:          []string{"GET"},
					APIVersion:          "2024-01-01",
					DeployabilityReason: "GET-only resource",
				},
			},
		}

		jsonStr, err := FormatChildrenAsJSON(result)
		require.NoError(t, err)

		// Verify it's valid JSON
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err)

		// Check structure
		assert.Contains(t, parsed, "deployable")
		assert.Contains(t, parsed, "filtered_out")

		deployable := parsed["deployable"].([]interface{})
		assert.Len(t, deployable, 1)

		filteredOut := parsed["filtered_out"].([]interface{})
		assert.Len(t, filteredOut, 1)
	})

	t.Run("handles empty results", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable:  []ChildResource{},
			FilteredOut: []ChildResource{},
		}

		jsonStr, err := FormatChildrenAsJSON(result)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err)

		assert.Empty(t, parsed["deployable"])
		assert.Empty(t, parsed["filtered_out"])
	})

	t.Run("handles nil result", func(t *testing.T) {
		jsonStr, err := FormatChildrenAsJSON(nil)
		require.NoError(t, err)
		assert.Equal(t, "{}", jsonStr)
	})

	t.Run("sorts by resource type", func(t *testing.T) {
		result := &ChildrenResult{
			Deployable: []ChildResource{
				{ResourceType: "Microsoft.App/managedEnvironments/storages", Operations: []string{"PUT"}, APIVersion: "2024-01-01", IsDeployable: true},
				{ResourceType: "Microsoft.App/managedEnvironments/certificates", Operations: []string{"PUT"}, APIVersion: "2024-01-01", IsDeployable: true},
			},
			FilteredOut: []ChildResource{},
		}

		jsonStr, err := FormatChildrenAsJSON(result)
		require.NoError(t, err)

		var parsed struct {
			Deployable []ChildResource `json:"deployable"`
		}
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err)

		require.Len(t, parsed.Deployable, 2)
		assert.Equal(t, "Microsoft.App/managedEnvironments/certificates", parsed.Deployable[0].ResourceType)
		assert.Equal(t, "Microsoft.App/managedEnvironments/storages", parsed.Deployable[1].ResourceType)
	})
}
