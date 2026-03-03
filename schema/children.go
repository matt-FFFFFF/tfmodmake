package schema

import (
	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
)

// ChildResource represents a child resource type discovered from the bicep-types index.
type ChildResource struct {
	// ResourceType is the fully qualified child resource type name.
	ResourceType string

	// APIVersions is the list of available API versions for this child resource.
	APIVersions []string
}

// DiscoverChildren finds child resources of a given parent resource type from the index.
// The depth parameter controls how many levels of nesting to include (1 = direct children only, 0 = unlimited).
func DiscoverChildren(idx *index.TypeIndex, parentType string, depth int) []ChildResource {
	entries := bicepdata.ListChildren(idx, parentType, depth)

	result := make([]ChildResource, len(entries))
	for i, entry := range entries {
		result[i] = ChildResource{
			ResourceType: entry.ResourceType,
			APIVersions:  entry.APIVersions,
		}
	}

	return result
}
