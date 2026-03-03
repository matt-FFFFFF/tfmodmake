package bicepdata

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
)

// ParseIndex parses raw index.json data into a TypeIndex.
func ParseIndex(data []byte) (*index.TypeIndex, error) {
	idx := index.NewTypeIndex()
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, fmt.Errorf("parsing index.json: %w", err)
	}
	return idx, nil
}

// LookupResource finds the types.json file path and type index for a given resource type and API version.
// It returns the CrossFileTypeReference which contains the RelativePath and Ref (type array index).
// The bicep-types-go index package may return either value or pointer CrossFileTypeReference
// depending on whether the index was built programmatically or deserialized from JSON.
func LookupResource(idx *index.TypeIndex, resourceType, apiVersion string) (*types.CrossFileTypeReference, error) {
	ref, ok := idx.GetResource(resourceType, apiVersion)
	if !ok {
		return nil, fmt.Errorf("resource %s@%s not found in index", resourceType, apiVersion)
	}

	// Handle both pointer and value types of CrossFileTypeReference.
	// BuildIndex and JSON unmarshaling produce value types, while programmatic
	// construction may use pointers.
	switch r := ref.(type) {
	case *types.CrossFileTypeReference:
		return r, nil
	case types.CrossFileTypeReference:
		return &r, nil
	default:
		return nil, fmt.Errorf("unexpected reference type %T for %s@%s: expected CrossFileTypeReference", ref, resourceType, apiVersion)
	}
}

// ListVersions returns all available API versions for a given resource type.
func ListVersions(idx *index.TypeIndex, resourceType string) []string {
	// The index stores resource types case-insensitively, so we need to search.
	for rt, versionMap := range idx.Resources {
		if strings.EqualFold(rt, resourceType) {
			versions := make([]string, 0, len(versionMap))
			for v := range versionMap {
				versions = append(versions, v)
			}
			return versions
		}
	}
	return nil
}

// ChildEntry represents a child resource type discovered from the index.
type ChildEntry struct {
	ResourceType string
	APIVersions  []string
}

// ListChildren enumerates child resource types from the index by matching
// the parent resource type prefix. The depth parameter controls how many
// additional path segments to include (1 = direct children only).
func ListChildren(idx *index.TypeIndex, parentType string, depth int) []ChildEntry {
	parentLower := strings.ToLower(parentType)
	parentSegments := strings.Count(parentLower, "/")

	childMap := make(map[string][]string) // resourceType -> []apiVersion

	for rt, versionMap := range idx.Resources {
		rtLower := strings.ToLower(rt)
		if !strings.HasPrefix(rtLower, parentLower+"/") {
			continue
		}

		// Check depth: count additional segments beyond parent
		childSegments := strings.Count(rtLower, "/")
		additionalDepth := childSegments - parentSegments
		if depth > 0 && additionalDepth > depth {
			continue
		}

		versions := make([]string, 0, len(versionMap))
		for v := range versionMap {
			versions = append(versions, v)
		}
		childMap[rt] = versions
	}

	result := make([]ChildEntry, 0, len(childMap))
	for rt, versions := range childMap {
		result = append(result, ChildEntry{
			ResourceType: rt,
			APIVersions:  versions,
		})
	}

	return result
}
