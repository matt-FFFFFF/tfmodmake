package bicepdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/index"
	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ResolveType ---

func TestResolveType_ValidTypeReference(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{
			&types.StringType{Pattern: "^[a-z]+$"},
			&types.IntegerType{},
			&types.BooleanType{},
		},
	}

	resolved, err := lr.ResolveType(&types.TypeReference{Ref: 0})
	require.NoError(t, err)
	st, ok := resolved.(*types.StringType)
	require.True(t, ok)
	assert.Equal(t, "^[a-z]+$", st.Pattern)

	resolved2, err := lr.ResolveType(&types.TypeReference{Ref: 2})
	require.NoError(t, err)
	assert.IsType(t, &types.BooleanType{}, resolved2)
}

func TestResolveType_OutOfBounds(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{&types.StringType{}},
	}

	_, err := lr.ResolveType(&types.TypeReference{Ref: 5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestResolveType_NegativeIndex(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{&types.StringType{}},
	}

	_, err := lr.ResolveType(&types.TypeReference{Ref: -1})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestResolveType_CrossFileReference(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{&types.StringType{}},
	}

	_, err := lr.ResolveType(&types.CrossFileTypeReference{RelativePath: "other.json", Ref: 0})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cross-file type references are not supported")
}

func TestResolveType_EmptyTypesArray(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{},
	}

	_, err := lr.ResolveType(&types.TypeReference{Ref: 0})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestResolveType_BoundaryIndex(t *testing.T) {
	lr := &LoadedResource{
		Types: []types.Type{
			&types.StringType{},
			&types.IntegerType{},
			&types.BooleanType{},
		},
	}

	// Last valid index
	resolved, err := lr.ResolveType(&types.TypeReference{Ref: 2})
	require.NoError(t, err)
	assert.IsType(t, &types.BooleanType{}, resolved)

	// One past the end
	_, err = lr.ResolveType(&types.TypeReference{Ref: 3})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

// --- isPreviewVersion ---

func TestIsPreviewVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"2025-01-01", false},
		{"2024-06-01", false},
		{"2024-01-01-preview", true},
		{"2024-01-01-Preview", true},
		{"2024-01-01-PREVIEW", true},
		{"2024-01-01-privatepreview", true},
		{"2024-01-01-beta", false},
		{"preview-2024-01-01", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			assert.Equal(t, tt.expected, isPreviewVersion(tt.version))
		})
	}
}

// --- resolveLatestVersion ---

func TestResolveLatestVersion_StableOnly(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2024-06-01",
		&types.CrossFileTypeReference{Ref: 0})
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{Ref: 1})
	idx.AddResource("Microsoft.App/containerApps", "2023-08-01",
		&types.CrossFileTypeReference{Ref: 2})

	version, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01", version)
}

func TestResolveLatestVersion_StablePreferredOverPreview(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2024-06-01",
		&types.CrossFileTypeReference{Ref: 0})
	idx.AddResource("Microsoft.App/containerApps", "2099-01-01-preview",
		&types.CrossFileTypeReference{Ref: 1})

	version, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	require.NoError(t, err)
	assert.Equal(t, "2024-06-01", version, "stable should be preferred over preview even when preview is newer")
}

func TestResolveLatestVersion_OnlyPreview_IncludePreview(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2024-01-01-preview",
		&types.CrossFileTypeReference{Ref: 0})
	idx.AddResource("Microsoft.App/containerApps", "2025-06-01-preview",
		&types.CrossFileTypeReference{Ref: 1})

	version, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", true)
	require.NoError(t, err)
	assert.Equal(t, "2025-06-01-preview", version)
}

func TestResolveLatestVersion_OnlyPreview_ExcludePreview(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2024-01-01-preview",
		&types.CrossFileTypeReference{Ref: 0})

	_, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no stable API versions found")
	assert.Contains(t, err.Error(), "--include-preview")
}

func TestResolveLatestVersion_NoVersionsFound(t *testing.T) {
	idx := index.NewTypeIndex()

	_, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no API versions found")
}

func TestResolveLatestVersion_MixedStableAndPreview(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2023-01-01",
		&types.CrossFileTypeReference{Ref: 0})
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{Ref: 1})
	idx.AddResource("Microsoft.App/containerApps", "2025-06-01-preview",
		&types.CrossFileTypeReference{Ref: 2})
	idx.AddResource("Microsoft.App/containerApps", "2024-01-01-preview",
		&types.CrossFileTypeReference{Ref: 3})

	// When includePreview is true, the overall latest version is returned
	// (preview 2025-06-01-preview > stable 2025-01-01).
	version, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", true)
	require.NoError(t, err)
	assert.Equal(t, "2025-06-01-preview", version)

	// When includePreview is false, the latest stable version is returned.
	version, err = resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01", version)
}

func TestResolveLatestVersion_SingleVersion(t *testing.T) {
	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{Ref: 0})

	version, err := resolveLatestVersion(idx, "Microsoft.App/containerApps", false)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01", version)
}

// --- LoadResourceFromIndex ---

func TestLoadResourceFromIndex_Success(t *testing.T) {
	// Build a types.json with a ResourceType at index 0 and an ObjectType at index 1
	rtObj := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-01-01",
		Body:           types.TypeReference{Ref: 1},
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	bodyObj := &types.ObjectType{
		Name:       "Microsoft.App/containerApps",
		Properties: map[string]types.ObjectTypeProperty{},
	}
	typesContent := buildTypesJSONLoader(t, rtObj, bodyObj)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{RelativePath: "microsoft.app/2025-01-01/types.json", Ref: 0})

	opts := &FetchOptions{BaseURL: srv.URL}
	loaded, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "2025-01-01", false, opts)
	require.NoError(t, err)

	assert.Equal(t, "Microsoft.App/containerApps@2025-01-01", loaded.ResourceType.Name)
	assert.Equal(t, "2025-01-01", loaded.APIVersion)
	assert.Equal(t, "Microsoft.App/containerApps", loaded.ResourceTypeName)
	assert.Len(t, loaded.Types, 2)
}

func TestLoadResourceFromIndex_AutoResolveVersion(t *testing.T) {
	rtObj := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-01-01",
		Body:           types.TypeReference{Ref: 1},
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	bodyObj := &types.ObjectType{
		Name:       "Microsoft.App/containerApps",
		Properties: map[string]types.ObjectTypeProperty{},
	}
	typesContent := buildTypesJSONLoader(t, rtObj, bodyObj)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2024-06-01",
		&types.CrossFileTypeReference{RelativePath: "microsoft.app/2024-06-01/types.json", Ref: 0})
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{RelativePath: "microsoft.app/2025-01-01/types.json", Ref: 0})

	opts := &FetchOptions{BaseURL: srv.URL}
	loaded, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "", false, opts)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01", loaded.APIVersion)
}

func TestLoadResourceFromIndex_TypeRefOutOfBounds(t *testing.T) {
	// types.json has only 1 element but ref points to index 5
	typesContent := buildTypesJSONLoader(t, &types.StringType{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{RelativePath: "types.json", Ref: 5})

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestLoadResourceFromIndex_TypeNotResourceType(t *testing.T) {
	// The ref points to a StringType instead of a ResourceType
	typesContent := buildTypesJSONLoader(t, &types.StringType{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{RelativePath: "types.json", Ref: 0})

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected *types.ResourceType")
}

func TestLoadResourceFromIndex_ResourceNotInIndex(t *testing.T) {
	idx := index.NewTypeIndex()
	opts := &FetchOptions{BaseURL: "http://unused.example.com"}
	_, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in index")
}

func TestLoadResourceFromIndex_FetchTypesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-01-01",
		&types.CrossFileTypeReference{RelativePath: "missing/types.json", Ref: 0})

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetching types")
}

func TestLoadResourceFromIndex_AutoResolvePreviewVersion(t *testing.T) {
	rtObj := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-06-01-preview",
		Body:           types.TypeReference{Ref: 1},
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	bodyObj := &types.ObjectType{
		Name:       "Microsoft.App/containerApps",
		Properties: map[string]types.ObjectTypeProperty{},
	}
	typesContent := buildTypesJSONLoader(t, rtObj, bodyObj)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	idx := index.NewTypeIndex()
	idx.AddResource("Microsoft.App/containerApps", "2025-06-01-preview",
		&types.CrossFileTypeReference{RelativePath: "types.json", Ref: 0})

	opts := &FetchOptions{BaseURL: srv.URL}

	// Without includePreview, should fail
	_, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no stable API versions")

	// With includePreview, should succeed
	loaded, err := LoadResourceFromIndex(context.Background(), idx, "Microsoft.App/containerApps", "", true, opts)
	require.NoError(t, err)
	assert.Equal(t, "2025-06-01-preview", loaded.APIVersion)
}

// --- LoadResource (full integration with FetchIndex) ---

func TestLoadResource_LocalFS(t *testing.T) {
	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, "generated")

	rtObj := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-01-01",
		Body:           types.TypeReference{Ref: 1},
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	bodyObj := &types.ObjectType{
		Name:       "Microsoft.App/containerApps",
		Properties: map[string]types.ObjectTypeProperty{},
	}
	typesContent := buildTypesJSONLoader(t, rtObj, bodyObj)

	// Write types.json
	typesDir := filepath.Join(genDir, "microsoft.app", "2025-01-01")
	require.NoError(t, os.MkdirAll(typesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(typesDir, "types.json"), typesContent, 0o644))

	// Build and write index.json.
	// Note: index JSON round-trip produces value-type CrossFileTypeReference,
	// but LookupResource asserts *CrossFileTypeReference. We construct the
	// index JSON manually using the format that the index package produces.
	indexJSON := `{
		"resources": {
			"Microsoft.App/containerApps@2025-01-01": {
				"$ref": "microsoft.app/2025-01-01/types.json#/0"
			}
		},
		"resourceFunctions": {}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(genDir, "index.json"), []byte(indexJSON), 0o644))

	opts := &FetchOptions{LocalPath: tmpDir}

	// LookupResource handles both pointer and value CrossFileTypeReference types,
	// so this should succeed even with JSON-deserialized index (which produces value types).
	result, err := LoadResource(context.Background(), "Microsoft.App/containerApps", "2025-01-01", false, opts)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01", result.APIVersion)
	assert.Equal(t, "Microsoft.App/containerApps", result.ResourceTypeName)
	assert.NotNil(t, result.ResourceType)
	assert.NotEmpty(t, result.Types)
}

func TestLoadResource_FetchIndexError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := LoadResource(context.Background(), "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fetching index")
}

func TestLoadResource_InvalidIndexJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := LoadResource(context.Background(), "Microsoft.App/containerApps", "2025-01-01", false, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing index")
}

// --- LoadedResource.ResolveType integration ---

func TestLoadedResource_ResolveBodyType(t *testing.T) {
	bodyObj := &types.ObjectType{
		Name: "MyResource",
		Properties: map[string]types.ObjectTypeProperty{
			"name": {
				Type:        &types.TypeReference{Ref: 2},
				Flags:       types.TypePropertyFlagsRequired,
				Description: "The resource name",
			},
		},
	}
	rtObj := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-01-01",
		Body:           &types.TypeReference{Ref: 1},
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	strType := &types.StringType{}

	lr := &LoadedResource{
		ResourceType:     rtObj,
		Types:            []types.Type{rtObj, bodyObj, strType},
		APIVersion:       "2025-01-01",
		ResourceTypeName: "Microsoft.App/containerApps",
	}

	// Resolve the body
	body, err := lr.ResolveType(rtObj.Body)
	require.NoError(t, err)
	obj, ok := body.(*types.ObjectType)
	require.True(t, ok)
	assert.Equal(t, "MyResource", obj.Name)

	// Resolve the name property type
	nameProp := obj.Properties["name"]
	nameType, err := lr.ResolveType(nameProp.Type)
	require.NoError(t, err)
	assert.IsType(t, &types.StringType{}, nameType)
}

func TestLoadedResource_ResolveChainedTypes(t *testing.T) {
	// Types: [StringType, ArrayType(->0), ObjectType(prop->1)]
	strType := &types.StringType{}
	arrType := &types.ArrayType{ItemType: types.TypeReference{Ref: 0}}
	objType := &types.ObjectType{
		Name: "Container",
		Properties: map[string]types.ObjectTypeProperty{
			"tags": {
				Type:  &types.TypeReference{Ref: 1},
				Flags: types.TypePropertyFlagsNone,
			},
		},
	}

	lr := &LoadedResource{
		Types: []types.Type{strType, arrType, objType},
	}

	// Resolve object
	resolved, err := lr.ResolveType(&types.TypeReference{Ref: 2})
	require.NoError(t, err)
	obj, ok := resolved.(*types.ObjectType)
	require.True(t, ok)

	// Resolve tags -> ArrayType
	tagsType, err := lr.ResolveType(obj.Properties["tags"].Type)
	require.NoError(t, err)
	at, ok := tagsType.(*types.ArrayType)
	require.True(t, ok)

	// Resolve array item -> StringType
	itemType, err := lr.ResolveType(&types.TypeReference{Ref: at.ItemType.(types.TypeReference).Ref})
	require.NoError(t, err)
	assert.IsType(t, &types.StringType{}, itemType)
}

// buildTypesJSONLoader is a helper to construct types.json content from type objects.
func buildTypesJSONLoader(t *testing.T, typeObjs ...types.Type) []byte {
	t.Helper()
	parts := make([]json.RawMessage, len(typeObjs))
	for i, obj := range typeObjs {
		b, err := obj.MarshalJSON()
		require.NoError(t, err)
		parts[i] = b
	}
	data, err := json.Marshal(parts)
	require.NoError(t, err)
	return data
}
