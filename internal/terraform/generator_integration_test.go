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

func TestAVMInterfacesScaffolding_ContainerAppsManagedEnvironments(t *testing.T) {
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

	// Generate AVM interfaces explicitly (since it's no longer included in base generation)
	err = GenerateInterfacesFile("Microsoft.App/managedEnvironments")
	require.NoError(t, err)

	// Check that main.interfaces.tf was generated
	interfacesBytes, err := os.ReadFile("main.interfaces.tf")
	require.NoError(t, err)
	interfacesContent := string(interfacesBytes)

	// Should reference the feat/prepv1 branch
	assert.Contains(t, interfacesContent, "terraform-azure-avm-utl-interfaces.git?ref=feat/prepv1")

	// Should wire mandatory IDs
	assert.Contains(t, interfacesContent, "this_resource_id")
	assert.Contains(t, interfacesContent, "azapi_resource.this.id")
	assert.Contains(t, interfacesContent, "parent_id")
	assert.Contains(t, interfacesContent, "var.parent_id")

	// Should pass interface inputs
	assert.Contains(t, interfacesContent, "role_assignments")
	assert.Contains(t, interfacesContent, "var.role_assignments")
	assert.Contains(t, interfacesContent, "lock")
	assert.Contains(t, interfacesContent, "var.lock")
	assert.Contains(t, interfacesContent, "diagnostic_settings")
	assert.Contains(t, interfacesContent, "var.diagnostic_settings")
	assert.Contains(t, interfacesContent, "private_endpoints")
	assert.Contains(t, interfacesContent, "local.private_endpoints")
	assert.Contains(t, interfacesContent, "private_endpoints_manage_dns_zone_group")
	assert.Contains(t, interfacesContent, "enable_telemetry")
	assert.Contains(t, interfacesContent, "var.enable_telemetry")
	assert.Contains(t, interfacesContent, "location")
	assert.Contains(t, interfacesContent, "var.location")

	// Check that variables.tf contains AVM interface variables
	variablesBytes, err := os.ReadFile("variables.tf")
	require.NoError(t, err)
	variablesContent := string(variablesBytes)

	assert.Contains(t, variablesContent, "variable \"enable_telemetry\"")
	assert.Contains(t, variablesContent, "variable \"diagnostic_settings\"")
	assert.Contains(t, variablesContent, "variable \"role_assignments\"")
	assert.Contains(t, variablesContent, "variable \"lock\"")
	assert.Contains(t, variablesContent, "variable \"private_endpoints\"")
	assert.Contains(t, variablesContent, "variable \"private_endpoints_manage_dns_zone_group\"")
	assert.Contains(t, variablesContent, "variable \"location\"")

	// Check that locals.tf contains private_endpoints local
	localsBytes, err := os.ReadFile("locals.tf")
	require.NoError(t, err)
	localsContent := string(localsBytes)

	assert.Contains(t, localsContent, "private_endpoints")
	// Should have the default subresource_name for managedEnvironments
	assert.Contains(t, localsContent, "managedEnvironments")
}

func TestAVMInterfacesScaffolding_AKSManagedClusters(t *testing.T) {
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

	// Generate AVM interfaces explicitly (since it's no longer included in base generation)
	err = GenerateInterfacesFile("Microsoft.ContainerService/managedClusters")
	require.NoError(t, err)

	// Check that main.interfaces.tf was generated
	interfacesBytes, err := os.ReadFile("main.interfaces.tf")
	require.NoError(t, err)
	interfacesContent := string(interfacesBytes)

	// Should reference the feat/prepv1 branch
	assert.Contains(t, interfacesContent, "terraform-azure-avm-utl-interfaces.git?ref=feat/prepv1")

	// Check that locals.tf contains private_endpoints local with AKS default
	localsBytes, err := os.ReadFile("locals.tf")
	require.NoError(t, err)
	localsContent := string(localsBytes)

	assert.Contains(t, localsContent, "private_endpoints")
	// Should have the default subresource_name for managedClusters
	assert.Contains(t, localsContent, "management")
}

func TestAVMInterfacesScaffolding_LocationAlwaysPresent(t *testing.T) {
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

	// Use a spec where location is not writable in the schema
	// This tests that location is always added as a variable
	specURL := "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/msi/resource-manager/Microsoft.ManagedIdentity/ManagedIdentity/preview/2025-01-31-preview/ManagedIdentity.json"

	doc, err := openapi.LoadSpec(specURL)
	require.NoError(t, err)

	schema, err := openapi.FindResource(doc, "Microsoft.ManagedIdentity/userAssignedIdentities")
	require.NoError(t, err)

	// Apply property writability overrides as done in main.go
	openapi.AnnotateSchemaRefOrigins(schema)
	if resolver, err := openapi.NewPropertyWritabilityResolver(specURL); err == nil && resolver != nil {
		openapi.ApplyPropertyWritabilityOverrides(schema, resolver)
	}

	supportsTags := SupportsTags(schema)
	supportsLocation := SupportsLocation(schema)
	apiVersion := doc.Info.Version

	err = Generate(schema, "Microsoft.ManagedIdentity/userAssignedIdentities", "resource_body", apiVersion, supportsTags, supportsLocation, nil)
	require.NoError(t, err)

	// Check that variables.tf contains location variable
	variablesBytes, err := os.ReadFile("variables.tf")
	require.NoError(t, err)
	variablesContent := string(variablesBytes)

	// Location should always be present (required by interfaces module)
	assert.Contains(t, variablesContent, "variable \"location\"")
}
