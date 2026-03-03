package bicepdata

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestIndex builds an index with resources pre-populated.
// Pointer-typed CrossFileTypeReferences are used because LookupResource
// performs a type assertion to *types.CrossFileTypeReference.
func newTestIndex(resources map[string]map[string]*types.CrossFileTypeReference) *index.TypeIndex {
	idx := index.NewTypeIndex()
	for rt, versions := range resources {
		for v, ref := range versions {
			idx.AddResource(rt, v, ref)
		}
	}
	return idx
}

// --- ParseIndex ---

func TestParseIndex_ValidJSON(t *testing.T) {
	// Build a TypeIndex, marshal it to JSON, then parse it back.
	orig := index.NewTypeIndex()
	orig.AddResource("Microsoft.App/containerApps", "2025-01-01",
		types.CrossFileTypeReference{RelativePath: "microsoft.app/2025-01-01/types.json", Ref: 5})
	orig.AddResource("Microsoft.Storage/storageAccounts", "2024-06-01",
		types.CrossFileTypeReference{RelativePath: "microsoft.storage/2024-06-01/types.json", Ref: 3})

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	parsed, err := ParseIndex(data)
	require.NoError(t, err)

	// After JSON round-trip, references are value types (not pointers).
	ref, ok := parsed.GetResource("Microsoft.App/containerApps", "2025-01-01")
	require.True(t, ok)
	crossRef, ok := ref.(types.CrossFileTypeReference)
	require.True(t, ok)
	assert.Equal(t, "microsoft.app/2025-01-01/types.json", crossRef.RelativePath)
	assert.Equal(t, 5, crossRef.Ref)

	ref2, ok := parsed.GetResource("Microsoft.Storage/storageAccounts", "2024-06-01")
	require.True(t, ok)
	crossRef2, ok := ref2.(types.CrossFileTypeReference)
	require.True(t, ok)
	assert.Equal(t, 3, crossRef2.Ref)
}

func TestParseIndex_MultipleVersionsSameResource(t *testing.T) {
	orig := index.NewTypeIndex()
	orig.AddResource("Microsoft.App/containerApps", "2025-01-01",
		types.CrossFileTypeReference{RelativePath: "a/types.json", Ref: 0})
	orig.AddResource("Microsoft.App/containerApps", "2024-06-01",
		types.CrossFileTypeReference{RelativePath: "b/types.json", Ref: 1})

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	parsed, err := ParseIndex(data)
	require.NoError(t, err)

	versionMap, ok := parsed.Resources["Microsoft.App/containerApps"]
	require.True(t, ok)
	assert.Len(t, versionMap, 2)
}

func TestParseIndex_EmptyResources(t *testing.T) {
	idx, err := ParseIndex([]byte(`{"resources":{}}`))
	require.NoError(t, err)
	assert.NotNil(t, idx)
	assert.Empty(t, idx.Resources)
}

func TestParseIndex_InvalidJSON(t *testing.T) {
	_, err := ParseIndex([]byte(`{invalid`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing index.json")
}

// --- LookupResource ---

func TestLookupResource_Found(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {RelativePath: "microsoft.app/2025-01-01/types.json", Ref: 5},
		},
	})

	ref, err := LookupResource(idx, "Microsoft.App/containerApps", "2025-01-01")
	require.NoError(t, err)
	assert.Equal(t, "microsoft.app/2025-01-01/types.json", ref.RelativePath)
	assert.Equal(t, 5, ref.Ref)
}

func TestLookupResource_NotFound_ResourceType(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {RelativePath: "types.json", Ref: 0},
		},
	})

	_, err := LookupResource(idx, "Microsoft.App/nonExistent", "2025-01-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in index")
}

func TestLookupResource_NotFound_APIVersion(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {RelativePath: "types.json", Ref: 0},
		},
	})

	_, err := LookupResource(idx, "Microsoft.App/containerApps", "2099-01-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in index")
}

func TestLookupResource_NonCrossFileRef(t *testing.T) {
	// Add a plain TypeReference (not CrossFileTypeReference) to the index.
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01", types.TypeReference{Ref: 0})

	_, err := LookupResource(idx, "Microsoft.App/containerApps", "2025-01-01")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected CrossFileTypeReference")
}

// --- ListVersions ---

func TestListVersions_Found(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01":         {RelativePath: "a/types.json", Ref: 0},
			"2024-06-01":         {RelativePath: "b/types.json", Ref: 0},
			"2024-01-01-preview": {RelativePath: "c/types.json", Ref: 0},
		},
	})

	versions := ListVersions(idx, "Microsoft.App/containerApps")
	sort.Strings(versions)
	assert.Equal(t, []string{"2024-01-01-preview", "2024-06-01", "2025-01-01"}, versions)
}

func TestListVersions_CaseInsensitive(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {RelativePath: "types.json", Ref: 0},
		},
	})

	// Query with different casing
	versions := ListVersions(idx, "microsoft.app/containerapps")
	require.Len(t, versions, 1)
	assert.Equal(t, "2025-01-01", versions[0])
}

func TestListVersions_NotFound(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {RelativePath: "types.json", Ref: 0},
		},
	})

	versions := ListVersions(idx, "Microsoft.Compute/virtualMachines")
	assert.Nil(t, versions)
}

func TestListVersions_EmptyIndex(t *testing.T) {
	idx := index.NewTypeIndex()
	versions := ListVersions(idx, "Microsoft.App/containerApps")
	assert.Nil(t, versions)
}

// --- ListChildren ---

func TestListChildren_DirectChildren(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/authConfigs": {
			"2025-01-01": {Ref: 1},
		},
		"Microsoft.App/containerApps/revisions": {
			"2025-01-01": {Ref: 2},
			"2024-06-01": {Ref: 3},
		},
		"Microsoft.App/containerApps/revisions/replicas": {
			"2025-01-01": {Ref: 4},
		},
		"Microsoft.App/managedEnvironments": {
			"2025-01-01": {Ref: 5},
		},
	})

	children := ListChildren(idx, "Microsoft.App/containerApps", 1)
	assert.Len(t, children, 2, "should include authConfigs and revisions but not revisions/replicas")

	childNames := make(map[string]bool)
	for _, c := range children {
		childNames[c.ResourceType] = true
	}
	assert.True(t, childNames["Microsoft.App/containerApps/authConfigs"])
	assert.True(t, childNames["Microsoft.App/containerApps/revisions"])
	assert.False(t, childNames["Microsoft.App/containerApps/revisions/replicas"])
	assert.False(t, childNames["Microsoft.App/managedEnvironments"])
}

func TestListChildren_AllDepths(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/authConfigs": {
			"2025-01-01": {Ref: 1},
		},
		"Microsoft.App/containerApps/revisions/replicas": {
			"2025-01-01": {Ref: 2},
		},
	})

	// depth=0 means unlimited
	children := ListChildren(idx, "Microsoft.App/containerApps", 0)
	assert.Len(t, children, 2, "depth=0 should return all descendants")
}

func TestListChildren_NoChildren(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
	})

	children := ListChildren(idx, "Microsoft.App/containerApps", 1)
	assert.Empty(t, children)
}

func TestListChildren_CaseInsensitive(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/authConfigs": {
			"2025-01-01": {Ref: 1},
		},
	})

	children := ListChildren(idx, "microsoft.app/containerapps", 1)
	require.Len(t, children, 1)
	assert.Equal(t, "Microsoft.App/containerApps/authConfigs", children[0].ResourceType)
}

func TestListChildren_MultipleAPIVersions(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/revisions": {
			"2025-01-01": {Ref: 1},
			"2024-06-01": {Ref: 2},
		},
	})

	children := ListChildren(idx, "Microsoft.App/containerApps", 1)
	require.Len(t, children, 1)
	sort.Strings(children[0].APIVersions)
	assert.Equal(t, []string{"2024-06-01", "2025-01-01"}, children[0].APIVersions)
}

func TestListChildren_DepthFiltering(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/revisions": {
			"2025-01-01": {Ref: 1},
		},
		"Microsoft.App/containerApps/revisions/replicas": {
			"2025-01-01": {Ref: 2},
		},
		"Microsoft.App/containerApps/revisions/replicas/containers": {
			"2025-01-01": {Ref: 3},
		},
	})

	// depth=2 should include revisions and revisions/replicas but not replicas/containers
	children := ListChildren(idx, "Microsoft.App/containerApps", 2)
	childNames := make(map[string]bool)
	for _, c := range children {
		childNames[c.ResourceType] = true
	}
	assert.True(t, childNames["Microsoft.App/containerApps/revisions"])
	assert.True(t, childNames["Microsoft.App/containerApps/revisions/replicas"])
	assert.False(t, childNames["Microsoft.App/containerApps/revisions/replicas/containers"])
}

func TestListChildren_ParentNotInIndex(t *testing.T) {
	// Children can exist even if the parent isn't in the index.
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps/authConfigs": {
			"2025-01-01": {Ref: 0},
		},
	})

	children := ListChildren(idx, "Microsoft.App/containerApps", 1)
	require.Len(t, children, 1)
	assert.Equal(t, "Microsoft.App/containerApps/authConfigs", children[0].ResourceType)
}

func TestListChildren_NegativeDepthTreatedAsUnlimited(t *testing.T) {
	idx := newTestIndex(map[string]map[string]*types.CrossFileTypeReference{
		"Microsoft.App/containerApps/revisions": {
			"2025-01-01": {Ref: 0},
		},
		"Microsoft.App/containerApps/revisions/replicas": {
			"2025-01-01": {Ref: 1},
		},
	})

	// depth=-1 should behave like depth=0 (unlimited) since the condition is depth > 0
	children := ListChildren(idx, "Microsoft.App/containerApps", -1)
	assert.Len(t, children, 2)
}
