package terraform

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
	"github.com/zclconf/go-cty/cty"
	"strings"
)

// privateEndpointSubresourcesByResourceType provides a mapping of ARM resource types to their
// Private Link subresource names.
//
// Source: https://learn.microsoft.com/en-us/azure/private-link/private-endpoint-dns (Commercial).
var privateEndpointSubresourcesByResourceType = map[string][]string{
	// Source: https://learn.microsoft.com/en-us/azure/private-link/private-endpoint-dns
	// NOTE: Keys are normalized to strings.ToLower(resourceType).
	strings.ToLower("Microsoft.MachineLearningServices/workspaces"): {"amlworkspace"},
	strings.ToLower("Microsoft.MachineLearningServices/registries"):  {"amlregistry"},
	strings.ToLower("Microsoft.CognitiveServices/accounts"):          {"account"},
	strings.ToLower("Microsoft.BotService/botServices"):             {"Bot", "Token"},

	strings.ToLower("Microsoft.Synapse/workspaces"):       {"Sql", "SqlOnDemand", "Dev"},
	strings.ToLower("Microsoft.Synapse/privateLinkHubs"):  {"Web"},
	strings.ToLower("Microsoft.EventHub/namespaces"):      {"namespace"},
	strings.ToLower("Microsoft.ServiceBus/namespaces"):    {"namespace"},
	strings.ToLower("Microsoft.DataFactory/factories"):    {"dataFactory", "portal"},
	strings.ToLower("Microsoft.HDInsight/clusters"):       {"gateway", "headnode"},
	strings.ToLower("Microsoft.Kusto/Clusters"):           {"cluster"},
	strings.ToLower("Microsoft.PowerBI/privateLinkServicesForPowerBI"): {"tenant"},
	strings.ToLower("Microsoft.Databricks/workspaces"):    {"databricks_ui_api", "browser_authentication"},
	strings.ToLower("Microsoft.Fabric/privateLinkServicesForFabric"):   {"workspace"},

	strings.ToLower("Microsoft.Batch/batchAccounts"): {"batchAccount", "nodeManagement"},
	strings.ToLower("Microsoft.DesktopVirtualization/workspaces"): {"global", "feed"},
	strings.ToLower("Microsoft.DesktopVirtualization/hostpools"):  {"connection"},

	strings.ToLower("Microsoft.ContainerService/managedClusters"): {"management"},
	strings.ToLower("Microsoft.App/managedEnvironments"):          {"managedEnvironments"},
	strings.ToLower("Microsoft.ContainerRegistry/registries"):     {"registry"},

	strings.ToLower("Microsoft.Sql/servers"):          {"sqlServer"},
	strings.ToLower("Microsoft.Sql/managedInstances"): {"managedInstance"},
	strings.ToLower("Microsoft.DocumentDB/databaseAccounts"): {"Sql", "MongoDB", "Cassandra", "Gremlin", "Table", "Analytical"},
	strings.ToLower("Microsoft.DBforPostgreSQL/serverGroupsv2"): {"coordinator"},
	strings.ToLower("Microsoft.DocumentDB/mongoClusters"):       {"MongoCluster"},
	strings.ToLower("Microsoft.DBforPostgreSQL/servers"):        {"postgresqlServer"},
	strings.ToLower("Microsoft.DBforPostgreSQL/flexibleServers"): {"postgresqlServer"},
	strings.ToLower("Microsoft.DBforMySQL/servers"):              {"mysqlServer"},
	strings.ToLower("Microsoft.DBforMySQL/flexibleServers"):      {"mysqlServer"},
	strings.ToLower("Microsoft.DBforMariaDB/servers"):            {"mariadbServer"},
	strings.ToLower("Microsoft.Cache/Redis"):                     {"redisCache"},
	strings.ToLower("Microsoft.Cache/RedisEnterprise"):           {"redisEnterprise"},

	strings.ToLower("Microsoft.HybridCompute/privateLinkScopes"): {"hybridcompute"},

	strings.ToLower("Microsoft.EventGrid/topics"):            {"topic"},
	strings.ToLower("Microsoft.EventGrid/domains"):           {"domain"},
	strings.ToLower("Microsoft.EventGrid/namespaces"):        {"topic", "topicSpace"},
	strings.ToLower("Microsoft.EventGrid/partnerNamespaces"): {"partnernamespace"},
	strings.ToLower("Microsoft.ApiManagement/service"):       {"Gateway"},
	strings.ToLower("Microsoft.HealthcareApis/workspaces"):   {"healthcareworkspace"},

	strings.ToLower("Microsoft.Devices/IotHubs"):            {"iotHub"},
	strings.ToLower("Microsoft.Devices/ProvisioningServices"): {"iotDps"},
	strings.ToLower("Microsoft.DeviceUpdate/accounts"):      {"DeviceUpdate"},
	strings.ToLower("Microsoft.IoTCentral/IoTApps"):         {"iotApp"},
	strings.ToLower("Microsoft.DigitalTwins/digitalTwinsInstances"): {"API"},

	strings.ToLower("Microsoft.Media/mediaservices"): {"keydelivery", "liveevent", "streamingendpoint"},

	strings.ToLower("Microsoft.Automation/automationAccounts"): {"Webhook", "DSCAndHybridWorker"},
	strings.ToLower("Microsoft.RecoveryServices/vaults"):       {"AzureBackup", "AzureBackup_secondary", "AzureSiteRecovery"},
	strings.ToLower("Microsoft.Insights/privateLinkScopes"):    {"azuremonitor"},
	strings.ToLower("Microsoft.Purview/accounts"):              {"account", "portal", "platform"},
	strings.ToLower("Microsoft.Migrate/migrateProjects"):       {"Default"},
	strings.ToLower("Microsoft.Migrate/assessmentProjects"):    {"Default"},
	strings.ToLower("Microsoft.Authorization/resourceManagementPrivateLinks"): {"ResourceManagement"},
	strings.ToLower("Microsoft.Dashboard/grafana"):             {"grafana"},
	strings.ToLower("Microsoft.Monitor/accounts"):              {"prometheusMetrics"},

	strings.ToLower("Microsoft.KeyVault/vaults"):        {"vault"},
	strings.ToLower("Microsoft.KeyVault/managedHSMs"):   {"managedhsm"},
	strings.ToLower("Microsoft.AppConfiguration/configurationStores"): {"configurationStores"},
	strings.ToLower("Microsoft.Attestation/attestationProviders"):     {"standard"},

	strings.ToLower("Microsoft.Storage/storageAccounts"): {"blob", "blob_secondary", "table", "table_secondary", "queue", "queue_secondary", "file", "web", "web_secondary", "dfs", "dfs_secondary"},
	strings.ToLower("Microsoft.StorageSync/storageSyncServices"): {"afs"},
	strings.ToLower("Microsoft.Compute/diskAccesses"):           {"disks"},
	strings.ToLower("Microsoft.ElasticSan/elasticSans"):         {"volumegroup"},
	strings.ToLower("Microsoft.FileShares/fileShares"):          {"FileShare"},

	strings.ToLower("Microsoft.Search/searchServices"): {"searchService"},
	strings.ToLower("Microsoft.Relay/namespaces"):      {"namespace"},
	strings.ToLower("Microsoft.Web/sites"):             {"sites"},
	strings.ToLower("Microsoft.SignalRService/SignalR"):   {"signalr"},
	strings.ToLower("Microsoft.Web/staticSites"):          {"staticSites"},
	strings.ToLower("Microsoft.SignalRService/WebPubSub"): {"webpubsub"},
}

func privateEndpointDefaultSubresource(resourceType string) (string, bool) {
	subresources, ok := privateEndpointSubresourcesByResourceType[strings.ToLower(resourceType)]
	if !ok || len(subresources) != 1 {
		return "", false
	}
	return subresources[0], true
}

// generateInterfaces creates main.interfaces.tf with the AVM interfaces module wiring.
// Only includes interface wiring for capabilities with swagger evidence.
func generateInterfaces(resourceType string, caps openapi.InterfaceCapabilities) error {
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

	// Always include telemetry and location (basic AVM requirements)
	moduleBody.SetAttributeRaw("enable_telemetry", hclgen.TokensForTraversal("var", "enable_telemetry"))
	moduleBody.SetAttributeRaw("location", hclgen.TokensForTraversal("var", "location"))

	// Only wire private endpoints if swagger indicates support
	if caps.SupportsPrivateEndpoints {
		moduleBody.SetAttributeRaw("private_endpoints", hclgen.TokensForTraversal("local", "private_endpoints"))
		moduleBody.SetAttributeRaw("private_endpoints_manage_dns_zone_group", hclgen.TokensForTraversal("var", "private_endpoints_manage_dns_zone_group"))
	}

	// Only wire diagnostic settings if swagger indicates support
	if caps.SupportsDiagnostics {
		moduleBody.SetAttributeRaw("diagnostic_settings", hclgen.TokensForTraversal("var", "diagnostic_settings"))
	}

	// Only wire customer managed key if swagger indicates support
	if caps.SupportsCustomerManagedKey {
		moduleBody.SetAttributeRaw("customer_managed_key", hclgen.TokensForTraversal("var", "customer_managed_key"))
	}

	// Note: Lock and role_assignments are ARM-level capabilities not reliably detectable from specs.
	// They are intentionally omitted. Use a separate helper command to add them if needed.

	return hclgen.WriteFile("main.interfaces.tf", file)
}
