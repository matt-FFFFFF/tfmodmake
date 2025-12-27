package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseRawGitHubFileURL(t *testing.T) {
	owner, repo, ref, filePath, ok := parseRawGitHubFileURL("https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json")
	assert.True(t, ok)
	assert.Equal(t, "Azure", owner)
	assert.Equal(t, "azure-rest-api-specs", repo)
	assert.Equal(t, "main", ref)
	assert.Equal(t, "specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json", filePath)

	_, _, _, _, ok = parseRawGitHubFileURL("https://example.com/not-github.json")
	assert.False(t, ok)
}

func TestParseGitHubTreeDirURL(t *testing.T) {
	loc, err := parseGitHubTreeDirURL("https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview")
	assert.NoError(t, err)
	assert.Equal(t, "Azure", loc.Owner)
	assert.Equal(t, "azure-rest-api-specs", loc.Repo)
	assert.Equal(t, "main", loc.Ref)
	assert.Equal(t, "specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview", loc.Dir)

	_, err = parseGitHubTreeDirURL("https://github.com/Azure/azure-rest-api-specs/blob/main/readme.md")
	assert.Error(t, err)
}

func TestParseAPIVersionDatePrefix(t *testing.T) {
	tm, ok := parseAPIVersionDatePrefix("2025-05-01")
	assert.True(t, ok)
	assert.Equal(t, time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC), tm)

	tm, ok = parseAPIVersionDatePrefix("2024-04-01-preview")
	assert.True(t, ok)
	assert.Equal(t, time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), tm)

	_, ok = parseAPIVersionDatePrefix("7.4")
	assert.False(t, ok)
}

func TestPickLatestVersionDirName(t *testing.T) {
	items := []githubContentsItem{
		{Type: "dir", Name: "2024-03-01"},
		{Type: "dir", Name: "2025-05-01"},
		{Type: "dir", Name: "2025-01-01"},
		{Type: "file", Name: "openapi.json"},
	}
	assert.Equal(t, "2025-05-01", pickLatestVersionDirName(items))

	previewItems := []githubContentsItem{
		{Type: "dir", Name: "2024-04-01-preview"},
		{Type: "dir", Name: "2024-12-01-preview"},
		{Type: "dir", Name: "2021-11-01-preview"},
	}
	assert.Equal(t, "2024-12-01-preview", pickLatestVersionDirName(previewItems))

	lexicalFallback := []githubContentsItem{
		{Type: "dir", Name: "7.4"},
		{Type: "dir", Name: "7.6"},
		{Type: "dir", Name: "7.5"},
	}
	assert.Equal(t, "7.6", pickLatestVersionDirName(lexicalFallback))
}

func TestInferSiblingStabilityRoots(t *testing.T) {
	stable, preview, ok := inferSiblingStabilityRoots("specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview")
	assert.True(t, ok)
	assert.Equal(t, "specification/app/resource-manager/Microsoft.App/ContainerApps/stable", stable)
	assert.Equal(t, "specification/app/resource-manager/Microsoft.App/ContainerApps/preview", preview)

	stable, preview, ok = inferSiblingStabilityRoots("specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2025-05-01")
	assert.True(t, ok)
	assert.Equal(t, "specification/keyvault/resource-manager/Microsoft.KeyVault/stable", stable)
	assert.Equal(t, "specification/keyvault/resource-manager/Microsoft.KeyVault/preview", preview)

	_, _, ok = inferSiblingStabilityRoots("just/some/random/path")
	assert.False(t, ok)
}

func TestBuildGitHubContentsError_RateLimit(t *testing.T) {
	h := make(http.Header)
	h.Set("X-RateLimit-Remaining", "0")
	h.Set("X-RateLimit-Reset", "1730000000")
	err := buildGitHubContentsError(http.StatusForbidden, "403 Forbidden", h, `{"message":"API rate limit exceeded"}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GITHUB_TOKEN")
	assert.Contains(t, err.Error(), "rate limit")
	assert.Contains(t, err.Error(), "remaining=0")
	assert.Contains(t, err.Error(), "resets at")
}

func TestBuildGitHubContentsError_Generic(t *testing.T) {
	err := buildGitHubContentsError(http.StatusNotFound, "404 Not Found", make(http.Header), "nope")
	assert.Equal(t, "github contents request failed: 404 Not Found: nope", err.Error())
}
