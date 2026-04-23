package bicepdata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
)

// ErrNoStableVersions is returned when a resource type has only preview API versions
// and --include-preview was not specified.
type ErrNoStableVersions struct {
	ResourceType    string
	PreviewVersions []string
}

func (e *ErrNoStableVersions) Error() string {
	return fmt.Sprintf("no stable API versions found for %s (preview versions available: %s); use --include-preview to select one",
		e.ResourceType, strings.Join(e.PreviewVersions, ", "))
}

// IsErrNoStableVersions reports whether err (or any error in its chain) is an ErrNoStableVersions.
func IsErrNoStableVersions(err error) bool {
	var target *ErrNoStableVersions
	return errors.As(err, &target)
}

// LoadedResource contains a resolved resource type and its supporting type array.
type LoadedResource struct {
	// ResourceType is the resolved ResourceType entry from types.json.
	ResourceType *types.ResourceType

	// Types is the full type array from the types.json file, used to resolve type references.
	Types []types.Type

	// APIVersion is the resolved API version.
	APIVersion string

	// ResourceTypeName is the fully qualified resource type name (e.g. "Microsoft.App/containerApps").
	ResourceTypeName string
}

// LoadResource orchestrates loading a resource type: fetch index, lookup resource,
// fetch types.json, and return the resolved ResourceType.
// If apiVersion is empty, the latest stable version is selected (or latest preview if includePreview is true).
func LoadResource(ctx context.Context, resourceType, apiVersion string, includePreview bool, opts *FetchOptions) (*LoadedResource, error) {
	// Fetch and parse the index
	indexData, err := FetchIndex(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}

	idx, err := ParseIndex(indexData)
	if err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}

	return LoadResourceFromIndex(ctx, idx, resourceType, apiVersion, includePreview, opts)
}

// LoadResourceFromIndex loads a resource type using a pre-fetched index.
// This is useful when you need to perform multiple lookups against the same index.
func LoadResourceFromIndex(ctx context.Context, idx *index.TypeIndex, resourceType, apiVersion string, includePreview bool, opts *FetchOptions) (*LoadedResource, error) {
	// Resolve API version if not specified
	if apiVersion == "" {
		var err error
		apiVersion, err = resolveLatestVersion(idx, resourceType, includePreview)
		if err != nil {
			return nil, err
		}
	}

	// Look up the resource in the index
	crossRef, err := LookupResource(idx, resourceType, apiVersion)
	if err != nil {
		return nil, err
	}

	// Fetch and parse the types.json file
	typesArray, err := FetchTypes(ctx, crossRef.RelativePath, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching types for %s@%s: %w", resourceType, apiVersion, err)
	}

	// Resolve the ResourceType entry from the types array
	if crossRef.Ref < 0 || crossRef.Ref >= len(typesArray) {
		return nil, fmt.Errorf("type reference index %d out of bounds (array length %d) for %s@%s",
			crossRef.Ref, len(typesArray), resourceType, apiVersion)
	}

	rt, ok := typesArray[crossRef.Ref].(*types.ResourceType)
	if !ok {
		return nil, fmt.Errorf("type at index %d is %T, expected *types.ResourceType for %s@%s",
			crossRef.Ref, typesArray[crossRef.Ref], resourceType, apiVersion)
	}

	return &LoadedResource{
		ResourceType:     rt,
		Types:            typesArray,
		APIVersion:       apiVersion,
		ResourceTypeName: resourceType,
	}, nil
}

// ResolveType resolves a type reference within the loaded resource's type array.
// Handles both pointer and value types of TypeReference and CrossFileTypeReference,
// since the bicep-types-go library may produce either depending on context.
func (lr *LoadedResource) ResolveType(ref types.ITypeReference) (types.Type, error) {
	switch r := ref.(type) {
	case *types.TypeReference:
		if r.Ref < 0 || r.Ref >= len(lr.Types) {
			return nil, fmt.Errorf("type reference index %d out of bounds (array length %d)", r.Ref, len(lr.Types))
		}
		return lr.Types[r.Ref], nil
	case types.TypeReference:
		if r.Ref < 0 || r.Ref >= len(lr.Types) {
			return nil, fmt.Errorf("type reference index %d out of bounds (array length %d)", r.Ref, len(lr.Types))
		}
		return lr.Types[r.Ref], nil
	case *types.CrossFileTypeReference:
		// Cross-file references are not supported within a single LoadedResource context.
		return nil, fmt.Errorf("cross-file type references are not supported in this context (ref to %s#/%d)", r.RelativePath, r.Ref)
	case types.CrossFileTypeReference:
		return nil, fmt.Errorf("cross-file type references are not supported in this context (ref to %s#/%d)", r.RelativePath, r.Ref)
	default:
		return nil, fmt.Errorf("unknown type reference: %T", ref)
	}
}

// resolveLatestVersion finds the latest API version for a resource type.
// When includePreview is false, only stable versions are considered.
// When includePreview is true, both stable and preview versions are compared
// and the overall latest (by lexicographic sort) is returned.
func resolveLatestVersion(idx *index.TypeIndex, resourceType string, includePreview bool) (string, error) {
	versions := ListVersions(idx, resourceType)
	if len(versions) == 0 {
		return "", fmt.Errorf("no API versions found for resource type %s", resourceType)
	}

	// Separate stable and preview versions
	var stable, preview []string
	for _, v := range versions {
		if isPreviewVersion(v) {
			preview = append(preview, v)
		} else {
			stable = append(stable, v)
		}
	}

	// Sort in descending order (latest first)
	sort.Sort(sort.Reverse(sort.StringSlice(stable)))
	sort.Sort(sort.Reverse(sort.StringSlice(preview)))

	if includePreview {
		// Compare latest stable and latest preview, return whichever is newer.
		// API versions are date-based (YYYY-MM-DD[-preview]) so lexicographic
		// comparison works correctly.
		var candidates []string
		if len(stable) > 0 {
			candidates = append(candidates, stable[0])
		}
		if len(preview) > 0 {
			candidates = append(candidates, preview[0])
		}
		if len(candidates) == 0 {
			return "", fmt.Errorf("no API versions found for resource type %s", resourceType)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
		return candidates[0], nil
	}

	if len(stable) > 0 {
		return stable[0], nil
	}

	if len(preview) > 0 {
		return "", &ErrNoStableVersions{ResourceType: resourceType, PreviewVersions: preview}
	}

	return "", fmt.Errorf("no API versions found for resource type %s", resourceType)
}

// isPreviewVersion returns true if the API version string contains "preview".
func isPreviewVersion(version string) bool {
	return strings.Contains(strings.ToLower(version), "preview")
}
