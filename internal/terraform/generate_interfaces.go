package terraform

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

// privateEndpointSubresourceMap provides opinionated defaults for private endpoint subresource names
// based on ARM resource types. This helps avoid incorrect guesses for resources where the subresource
// name is not obvious.
var privateEndpointSubresourceMap = map[string]string{
	"Microsoft.App/managedEnvironments":             "managedEnvironment",
	"Microsoft.ContainerService/managedClusters":    "management",
	"Microsoft.KeyVault/vaults":                     "vault",
	"Microsoft.Storage/storageAccounts":             "blob", // Common default; users may override
	"Microsoft.Sql/servers":                         "sqlServer",
	"Microsoft.DBforPostgreSQL/flexibleServers":     "postgresqlServer",
	"Microsoft.DBforMySQL/flexibleServers":          "mysqlServer",
	"Microsoft.CognitiveServices/accounts":          "account",
	"Microsoft.ContainerRegistry/registries":        "registry",
	"Microsoft.EventHub/namespaces":                 "namespace",
	"Microsoft.ServiceBus/namespaces":               "namespace",
	"Microsoft.Web/sites":                           "sites",
	"Microsoft.DocumentDB/databaseAccounts":         "Sql", // Cosmos DB
	"Microsoft.Cache/redis":                         "redisCache",
	"Microsoft.Network/applicationGateways":         "applicationGateway",
	"Microsoft.Insights/components":                 "appInsights",
	"Microsoft.Search/searchServices":               "searchService",
	"Microsoft.SignalRService/signalR":              "signalR",
	"Microsoft.Devices/IotHubs":                     "iotHub",
	"Microsoft.ApiManagement/service":               "Gateway",
}

// generateInterfaces creates main.interfaces.tf with the AVM interfaces module wiring.
func generateInterfaces(resourceType string) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	// Add module block for avm_interfaces
	moduleBlock := body.AppendNewBlock("module", []string{"avm_interfaces"})
	moduleBody := moduleBlock.Body()

	// Source the interfaces module from Git + branch
	moduleBody.SetAttributeValue("source", cty.StringVal("git::https://github.com/Azure/terraform-azure-avm-utl-interfaces.git?ref=feat/prepv1"))

	// Wire mandatory IDs
	moduleBody.SetAttributeRaw("parent_id", hclgen.TokensForTraversal("var", "parent_id"))
	moduleBody.SetAttributeRaw("this_resource_id", hclgen.TokensForTraversal("azapi_resource", "this", "id"))

	// Pass interface inputs
	moduleBody.SetAttributeRaw("role_assignments", hclgen.TokensForTraversal("var", "role_assignments"))
	moduleBody.SetAttributeRaw("lock", hclgen.TokensForTraversal("var", "lock"))
	moduleBody.SetAttributeRaw("diagnostic_settings", hclgen.TokensForTraversal("var", "diagnostic_settings"))
	moduleBody.SetAttributeRaw("private_endpoints", hclgen.TokensForTraversal("local", "private_endpoints"))
	moduleBody.SetAttributeRaw("private_endpoints_scope", hclgen.TokensForTraversal("azapi_resource", "this", "id"))
	moduleBody.SetAttributeRaw("private_endpoints_manage_dns_zone_group", hclgen.TokensForTraversal("var", "private_endpoints_manage_dns_zone_group"))
	moduleBody.SetAttributeRaw("enable_telemetry", hclgen.TokensForTraversal("var", "enable_telemetry"))
	moduleBody.SetAttributeRaw("location", hclgen.TokensForTraversal("var", "location"))

	return hclgen.WriteFile("main.interfaces.tf", file)
}
