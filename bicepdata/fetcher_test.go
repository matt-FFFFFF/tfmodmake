package bicepdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/bicep-types/src/bicep-types-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTypesJSON constructs a valid types.json byte slice from Go type objects.
func buildTypesJSON(t *testing.T, typeObjs ...types.Type) []byte {
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

func TestDeserializeTypes_StringType(t *testing.T) {
	data := buildTypesJSON(t, &types.StringType{Pattern: "^[a-z]+$"})

	result, err := DeserializeTypes(data)
	require.NoError(t, err)
	require.Len(t, result, 1)

	st, ok := result[0].(*types.StringType)
	require.True(t, ok, "expected *types.StringType, got %T", result[0])
	assert.Equal(t, "^[a-z]+$", st.Pattern)
}

func TestDeserializeTypes_MultipleMixedTypes(t *testing.T) {
	minLen := int64(1)
	data := buildTypesJSON(t,
		&types.StringType{MinLength: &minLen},
		&types.IntegerType{},
		&types.BooleanType{},
		&types.ObjectType{Name: "TestObj", Properties: map[string]types.ObjectTypeProperty{}},
		&types.AnyType{},
		&types.NullType{},
		&types.StringLiteralType{Value: "Enabled"},
	)

	result, err := DeserializeTypes(data)
	require.NoError(t, err)
	require.Len(t, result, 7)

	assert.IsType(t, &types.StringType{}, result[0])
	assert.IsType(t, &types.IntegerType{}, result[1])
	assert.IsType(t, &types.BooleanType{}, result[2])
	assert.IsType(t, &types.ObjectType{}, result[3])
	assert.IsType(t, &types.AnyType{}, result[4])
	assert.IsType(t, &types.NullType{}, result[5])
	assert.IsType(t, &types.StringLiteralType{}, result[6])

	sl := result[6].(*types.StringLiteralType)
	assert.Equal(t, "Enabled", sl.Value)
}

func TestDeserializeTypes_ResourceType(t *testing.T) {
	rt := &types.ResourceType{
		Name:           "Microsoft.App/containerApps@2025-01-01",
		Body:           types.TypeReference{Ref: 1},
		ReadableScopes: types.ScopeTypeResourceGroup,
		WritableScopes: types.ScopeTypeResourceGroup,
	}
	data := buildTypesJSON(t, rt)

	result, err := DeserializeTypes(data)
	require.NoError(t, err)
	require.Len(t, result, 1)

	parsed, ok := result[0].(*types.ResourceType)
	require.True(t, ok)
	assert.Equal(t, "Microsoft.App/containerApps@2025-01-01", parsed.Name)
	assert.Equal(t, types.ScopeTypeResourceGroup, parsed.WritableScopes)
}

func TestDeserializeTypes_ArrayAndUnionTypes(t *testing.T) {
	data := buildTypesJSON(t,
		&types.ArrayType{ItemType: types.TypeReference{Ref: 0}},
		&types.UnionType{Elements: []types.ITypeReference{
			types.TypeReference{Ref: 0},
			types.TypeReference{Ref: 1},
		}},
	)

	result, err := DeserializeTypes(data)
	require.NoError(t, err)
	require.Len(t, result, 2)

	at, ok := result[0].(*types.ArrayType)
	require.True(t, ok)
	assert.NotNil(t, at.ItemType)

	ut, ok := result[1].(*types.UnionType)
	require.True(t, ok)
	assert.Len(t, ut.Elements, 2)
}

func TestDeserializeTypes_InvalidJSON(t *testing.T) {
	_, err := DeserializeTypes([]byte(`not json`))
	assert.Error(t, err)
}

func TestDeserializeTypes_InvalidTypeInArray(t *testing.T) {
	_, err := DeserializeTypes([]byte(`[{"$type":"UnknownType"}]`))
	assert.Error(t, err)
}

func TestDeserializeTypes_EmptyArray(t *testing.T) {
	result, err := DeserializeTypes([]byte(`[]`))
	require.NoError(t, err)
	assert.Empty(t, result)
}

// --- FetchIndex / FetchTypes with local filesystem ---

func TestFetchIndex_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, "generated")
	require.NoError(t, os.MkdirAll(genDir, 0o755))

	indexContent := []byte(`{"resources":{}}`)
	require.NoError(t, os.WriteFile(filepath.Join(genDir, "index.json"), indexContent, 0o644))

	opts := &FetchOptions{LocalPath: tmpDir}
	data, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, indexContent, data)
}

func TestFetchTypes_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	typesDir := filepath.Join(tmpDir, "generated", "microsoft.app", "2025-01-01")
	require.NoError(t, os.MkdirAll(typesDir, 0o755))

	typesContent := buildTypesJSON(t, &types.StringType{})
	require.NoError(t, os.WriteFile(filepath.Join(typesDir, "types.json"), typesContent, 0o644))

	opts := &FetchOptions{LocalPath: tmpDir}
	result, err := FetchTypes(context.Background(), "microsoft.app/2025-01-01/types.json", opts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.IsType(t, &types.StringType{}, result[0])
}

func TestFetchIndex_LocalPathNotFound(t *testing.T) {
	opts := &FetchOptions{LocalPath: "/nonexistent/path"}
	_, err := FetchIndex(context.Background(), opts)
	assert.Error(t, err)
}

// --- Local path takes priority over remote ---

func TestFetchIndex_LocalPathPreferredOverRemote(t *testing.T) {
	// Set up a local path with valid content
	tmpDir := t.TempDir()
	genDir := filepath.Join(tmpDir, "generated")
	require.NoError(t, os.MkdirAll(genDir, 0o755))
	localContent := []byte(`{"resources":{"local":"yes"}}`)
	require.NoError(t, os.WriteFile(filepath.Join(genDir, "index.json"), localContent, 0o644))

	// Set up an HTTP server that would return different content
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP server should not have been called when LocalPath is set")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resources":{"remote":"yes"}}`))
	}))
	defer srv.Close()

	opts := &FetchOptions{
		LocalPath: tmpDir,
		BaseURL:   srv.URL,
	}
	data, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, localContent, data)
}

// --- HTTP download tests ---

func TestFetchIndex_HTTPDownload(t *testing.T) {
	indexData := []byte(`{"resources":{}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/index.json", r.URL.Path)
		assert.Equal(t, defaultUserAgent, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL}
	data, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, indexData, data)
}

func TestFetchIndex_HTTPWithGitHubToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token my-secret-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL, GitHubToken: "my-secret-token"}
	_, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
}

func TestFetchIndex_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := FetchIndex(context.Background(), opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestFetchTypes_HTTPDownload(t *testing.T) {
	typesContent := buildTypesJSON(t, &types.BooleanType{}, &types.IntegerType{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/microsoft.app/2025-01-01/types.json", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL}
	result, err := FetchTypes(context.Background(), "microsoft.app/2025-01-01/types.json", opts)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.IsType(t, &types.BooleanType{}, result[0])
	assert.IsType(t, &types.IntegerType{}, result[1])
}

// --- Cache tests ---

func TestFetchIndex_CacheWrite(t *testing.T) {
	indexData := []byte(`{"resources":{"cached":"data"}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	opts := &FetchOptions{BaseURL: srv.URL, CacheDir: cacheDir}

	data, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, indexData, data)

	// Verify the file was cached
	cachedData, err := os.ReadFile(filepath.Join(cacheDir, "index.json"))
	require.NoError(t, err)
	assert.Equal(t, indexData, cachedData)
}

func TestFetchIndex_CacheRead(t *testing.T) {
	cacheDir := t.TempDir()
	cachedContent := []byte(`{"resources":{"from":"cache"}}`)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "index.json"), cachedContent, 0o644))

	// HTTP server should never be called
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP server should not have been called when cache has data")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resources":{"from":"remote"}}`))
	}))
	defer srv.Close()

	opts := &FetchOptions{BaseURL: srv.URL, CacheDir: cacheDir}
	data, err := FetchIndex(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, cachedContent, data)
}

func TestFetchTypes_CacheWriteNestedPath(t *testing.T) {
	typesContent := buildTypesJSON(t, &types.StringType{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(typesContent)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	opts := &FetchOptions{BaseURL: srv.URL, CacheDir: cacheDir}

	relPath := "microsoft.app/2025-01-01/types.json"
	result, err := FetchTypes(context.Background(), relPath, opts)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// Verify nested cache file was created
	cachedData, err := os.ReadFile(filepath.Join(cacheDir, relPath))
	require.NoError(t, err)
	assert.Equal(t, typesContent, cachedData)
}

// --- FetchOptions helper methods ---

func TestFetchOptions_BaseURL_Default(t *testing.T) {
	var opts *FetchOptions
	assert.Equal(t, defaultBaseURL, opts.baseURL())
}

func TestFetchOptions_BaseURL_Override(t *testing.T) {
	opts := &FetchOptions{BaseURL: "https://example.com/types"}
	assert.Equal(t, "https://example.com/types", opts.baseURL())
}

func TestFetchOptions_HTTPClient_Default(t *testing.T) {
	var opts *FetchOptions
	client := opts.httpClient()
	assert.NotNil(t, client)
	assert.Equal(t, defaultTimeout, client.Timeout)
}

func TestFetchOptions_HTTPClient_Override(t *testing.T) {
	custom := &http.Client{}
	opts := &FetchOptions{HTTPClient: custom}
	assert.Same(t, custom, opts.httpClient())
}

// --- Context cancellation ---

func TestFetchIndex_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	opts := &FetchOptions{BaseURL: srv.URL}
	_, err := FetchIndex(ctx, opts)
	assert.Error(t, err)
}
