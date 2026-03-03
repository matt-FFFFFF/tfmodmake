// Package bicepdata provides functions to download and cache bicep-types-az type definitions.
package bicepdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/bicep-types/src/bicep-types-go/types"
)

const (
	// defaultBaseURL is the raw GitHub URL for the bicep-types-az generated output.
	defaultBaseURL = "https://raw.githubusercontent.com/Azure/bicep-types-az/main/generated"

	// defaultUserAgent identifies this tool in HTTP requests.
	defaultUserAgent = "tfmodmake/1.0"

	// defaultTimeout is the HTTP request timeout.
	defaultTimeout = 60 * time.Second
)

// FetchOptions configures how types are fetched.
type FetchOptions struct {
	// LocalPath overrides remote fetching with a local filesystem path
	// to a bicep-types-az checkout. When set, files are read from
	// {LocalPath}/generated/index.json and {LocalPath}/generated/{path}/types.json.
	LocalPath string

	// CacheDir is an optional directory for caching downloaded type files.
	// If empty, no caching is performed.
	CacheDir string

	// GitHubToken is an optional GitHub token for authenticated requests.
	// This helps avoid rate limiting.
	GitHubToken string

	// BaseURL overrides the default GitHub raw content URL.
	// Useful for testing or using a mirror.
	BaseURL string

	// HTTPClient overrides the default HTTP client.
	HTTPClient *http.Client
}

func (o *FetchOptions) baseURL() string {
	if o != nil && o.BaseURL != "" {
		return o.BaseURL
	}
	return defaultBaseURL
}

func (o *FetchOptions) httpClient() *http.Client {
	if o != nil && o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: defaultTimeout}
}

// FetchIndex downloads and parses the bicep-types-az index.json file.
// The index maps resource types and API versions to their types.json file paths.
func FetchIndex(ctx context.Context, opts *FetchOptions) ([]byte, error) {
	return fetchFile(ctx, "index.json", opts)
}

// FetchTypes downloads and parses a specific types.json file.
// The relativePath is the path relative to the generated/ directory,
// e.g. "microsoft.app/2025-01-01/types.json".
func FetchTypes(ctx context.Context, relativePath string, opts *FetchOptions) ([]types.Type, error) {
	data, err := fetchFile(ctx, relativePath, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching types file %s: %w", relativePath, err)
	}

	return DeserializeTypes(data)
}

// DeserializeTypes parses a types.json byte slice into a slice of typed objects.
func DeserializeTypes(data []byte) ([]types.Type, error) {
	var rawTypes []json.RawMessage
	if err := json.Unmarshal(data, &rawTypes); err != nil {
		return nil, fmt.Errorf("parsing types array: %w", err)
	}

	result := make([]types.Type, len(rawTypes))
	for i, raw := range rawTypes {
		t, err := types.UnmarshalType(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling type at index %d: %w", i, err)
		}
		result[i] = t
	}

	return result, nil
}

// fetchFile retrieves a file either from the local filesystem or via HTTP.
func fetchFile(ctx context.Context, relativePath string, opts *FetchOptions) ([]byte, error) {
	// Try local filesystem first
	if opts != nil && opts.LocalPath != "" {
		return readLocalFile(filepath.Join(opts.LocalPath, "generated", relativePath))
	}

	// Try cache
	if opts != nil && opts.CacheDir != "" {
		cached, err := readCachedFile(opts.CacheDir, relativePath)
		if err == nil {
			return cached, nil
		}
	}

	// Download from remote
	data, err := downloadFile(ctx, relativePath, opts)
	if err != nil {
		return nil, err
	}

	// Write to cache
	if opts != nil && opts.CacheDir != "" {
		_ = writeCacheFile(opts.CacheDir, relativePath, data)
	}

	return data, nil
}

func readLocalFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading local file %s: %w", path, err)
	}
	return data, nil
}

func readCachedFile(cacheDir, relativePath string) ([]byte, error) {
	cachePath := filepath.Join(cacheDir, relativePath)
	return os.ReadFile(cachePath)
}

func writeCacheFile(cacheDir, relativePath string, data []byte) error {
	cachePath := filepath.Join(cacheDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cachePath, data, 0o644)
}

func downloadFile(ctx context.Context, relativePath string, opts *FetchOptions) ([]byte, error) {
	url := opts.baseURL() + "/" + relativePath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	if opts != nil && opts.GitHubToken != "" {
		req.Header.Set("Authorization", "token "+opts.GitHubToken)
	}

	client := opts.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", url, err)
	}

	return data, nil
}
