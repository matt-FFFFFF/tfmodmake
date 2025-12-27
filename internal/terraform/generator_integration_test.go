package terraform

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
)

func TestSupportsLocation_ManagedIdentityUserAssigned(t *testing.T) {
	t.Parallel()

	specURL := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/62f4b6969f4273d444daec4a1d2bf9769820fca2/specification/msi/resource-manager/Microsoft.ManagedIdentity/ManagedIdentity/preview/2025-01-31-preview/ManagedIdentity.json"

	doc, err := openapi.LoadSpec(specURL)
	require.NoError(t, err)

	schema, err := openapi.FindResource(doc, "Microsoft.ManagedIdentity/userAssignedIdentities")
	require.NoError(t, err)

	assert.True(t, SupportsLocation(schema), "userAssignedIdentities should support location via TrackedResource inheritance")
}

func TestResponseExportValues_ContainerAppsManagedEnvironments(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	// Note: t.Parallel() removed due to os.Chdir race condition

	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	specURL := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json"

	doc, err := openapi.LoadSpec(specURL)
	require.NoError(t, err)

	schema, err := openapi.FindResource(doc, "Microsoft.App/managedEnvironments")
	require.NoError(t, err)

	// Apply property writability overrides as done in main.go
	openapi.AnnotateSchemaRefOrigins(schema)
	if resolver, err := openapi.NewPropertyWritabilityResolver(specURL); err == nil && resolver != nil {
		openapi.ApplyPropertyWritabilityOverrides(schema, resolver)
	}

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)
	apiVersion := doc.Info.Version

	err = Generate(schema, "Microsoft.App/managedEnvironments", "resource_body", apiVersion, supportsTags, supportsLocation, nil)
	require.NoError(t, err)

	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	mainContent := string(mainBytes)

	// Should have response_export_values populated
	assert.Contains(t, mainContent, "response_export_values")

	// Should include expected readOnly fields
	assert.Contains(t, mainContent, "properties.defaultDomain")
	assert.Contains(t, mainContent, "properties.staticIp")
	assert.Contains(t, mainContent, "properties.provisioningState")
	assert.Contains(t, mainContent, "identity.principalId")

	// Should have the comment about trimming
	assert.Contains(t, mainContent, "Trim response_export_values")

	// Should NOT contain array-indexed paths (blocklist)
	// Note: We can't check for specific array paths since the spec may not have them,
	// but we verify the filtering logic works via unit tests
}

func TestResponseExportValues_AKSManagedClusters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	// Note: t.Parallel() removed due to os.Chdir race condition

	tmpDir := t.TempDir()
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	specURL := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/containerservice/resource-manager/Microsoft.ContainerService/aks/stable/2025-10-01/managedClusters.json"

	doc, err := openapi.LoadSpec(specURL)
	require.NoError(t, err)

	schema, err := openapi.FindResource(doc, "Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)

	// Apply property writability overrides as done in main.go
	openapi.AnnotateSchemaRefOrigins(schema)
	if resolver, err := openapi.NewPropertyWritabilityResolver(specURL); err == nil && resolver != nil {
		openapi.ApplyPropertyWritabilityOverrides(schema, resolver)
	}

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)
	apiVersion := doc.Info.Version

	err = Generate(schema, "Microsoft.ContainerService/managedClusters", "resource_body", apiVersion, supportsTags, supportsLocation, nil)
	require.NoError(t, err)

	mainBytes, err := os.ReadFile("main.tf")
	require.NoError(t, err)
	mainContent := string(mainBytes)

	// Should have response_export_values populated
	assert.Contains(t, mainContent, "response_export_values")

	// Should include expected readOnly fields
	assert.Contains(t, mainContent, "properties.fqdn")
	assert.Contains(t, mainContent, "properties.provisioningState")

	// Should NOT contain array-indexed paths (the blocklist should filter them out)
	// Since agentPoolProfiles is an array, any indexed access should be blocked
	assert.NotContains(t, mainContent, "[0]")
	assert.NotContains(t, mainContent, "[1]")

	// Should NOT contain .status. paths
	assert.NotRegexp(t, `\.status\.`, mainContent)

	// Should NOT contain .provisioningError. paths
	assert.NotRegexp(t, `\.provisioningError\.`, mainContent)
}
