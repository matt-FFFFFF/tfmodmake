package terraform

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
	"github.com/zclconf/go-cty/cty"
)

// This file contains AVM (Azure Verified Modules) interface variable generation.
// These are standard variables that follow AVM patterns for customer_managed_key,
// diagnostic_settings, private_endpoints, etc.

// emitCustomerManagedKeyVar generates the customer_managed_key variable if supported.
func emitCustomerManagedKeyVar(body *hclwrite.Body, caps openapi.InterfaceCapabilities, appendVariable func(string, string, hclwrite.Tokens) *hclwrite.Body, appendTFLintIgnoreUnused func()) {
	if !caps.SupportsCustomerManagedKey {
		return
	}
	
	appendTFLintIgnoreUnused()
	cmkBody := appendVariable(
		"customer_managed_key",
		"A map describing customer-managed keys to associate with the resource.",
		hclwrite.TokensForFunctionCall(
			"object",
			hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
				{Name: hclwrite.TokensForIdentifier("key_vault_resource_id"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("key_name"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("key_version"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
				{Name: hclwrite.TokensForIdentifier("user_assigned_identity"), Value: hclwrite.TokensForFunctionCall(
					"optional",
					hclwrite.TokensForFunctionCall(
						"object",
						hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
							{Name: hclwrite.TokensForIdentifier("resource_id"), Value: hclwrite.TokensForIdentifier("string")},
						}),
					),
					hclwrite.TokensForIdentifier("null"),
				)},
			}),
		),
	)
	cmkBody.SetAttributeRaw("default", hclwrite.TokensForIdentifier("null"))
	body.AppendNewline()
}

// emitEnableTelemetryVar generates the enable_telemetry variable (always included for AVM compliance).
func emitEnableTelemetryVar(body *hclwrite.Body, appendVariable func(string, string, hclwrite.Tokens) *hclwrite.Body) {
	telemetryBody := appendVariable(
		"enable_telemetry",
		"This variable controls whether or not telemetry is enabled for the module. For more information see https://aka.ms/avm/telemetryinfo.",
		hclwrite.TokensForIdentifier("bool"),
	)
	telemetryBody.SetAttributeValue("default", cty.True)
	telemetryBody.SetAttributeValue("nullable", cty.False)
	body.AppendNewline()
}

// emitDiagnosticSettingsVar generates the diagnostic_settings variable with validations if supported.
func emitDiagnosticSettingsVar(body *hclwrite.Body, caps openapi.InterfaceCapabilities, appendVariable func(string, string, hclwrite.Tokens) *hclwrite.Body) {
	if !caps.SupportsDiagnostics {
		return
	}
	
	diagBody := appendVariable(
		"diagnostic_settings",
		"A map of diagnostic settings to create on the resource.",
		hclwrite.TokensForFunctionCall("map", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("log_categories"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("set", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.ListValEmpty(cty.String)))},
			{Name: hclwrite.TokensForIdentifier("log_groups"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("set", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.ListVal([]cty.Value{cty.StringVal("allLogs")})))},
			{Name: hclwrite.TokensForIdentifier("metric_categories"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("set", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.ListVal([]cty.Value{cty.StringVal("AllMetrics")})))},
			{Name: hclwrite.TokensForIdentifier("log_analytics_destination_type"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForValue(cty.StringVal("Dedicated")))},
			{Name: hclwrite.TokensForIdentifier("workspace_resource_id"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("storage_account_resource_id"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("event_hub_authorization_rule_resource_id"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("event_hub_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("marketplace_partner_resource_id"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
		}))),
	)
	diagBody.SetAttributeValue("default", cty.MapValEmpty(cty.DynamicPseudoType))
	diagBody.SetAttributeValue("nullable", cty.False)
	
	// Add validations
	addDiagnosticSettingsValidations(diagBody)
	body.AppendNewline()
}

// addDiagnosticSettingsValidations adds the two validation blocks for diagnostic_settings.
func addDiagnosticSettingsValidations(diagBody *hclwrite.Body) {
	// Validation 1: log_analytics_destination_type must be one of allowed values
	{
		validation := diagBody.AppendNewBlock("validation", nil)
		validationBody := validation.Body()

		varDS := hclgen.TokensForTraversal("var", "diagnostic_settings")
		containsCall := hclwrite.TokensForFunctionCall(
			"contains",
			hclwrite.TokensForValue(cty.ListVal([]cty.Value{cty.StringVal("Dedicated"), cty.StringVal("AzureDiagnostics")})),
			hclgen.TokensForTraversal("v", "log_analytics_destination_type"),
		)

		// alltrue([for _, v in var.diagnostic_settings : contains([...], v.log_analytics_destination_type)])
		listComp := hclwrite.Tokens{
			&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("for")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("_")},
			&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("v")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in")},
		}
		listComp = append(listComp, varDS...)
		listComp = append(listComp, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
		listComp = append(listComp, containsCall...)
		listComp = append(listComp, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})

		validationBody.SetAttributeRaw("condition", hclwrite.TokensForFunctionCall("alltrue", listComp))
		validationBody.SetAttributeValue("error_message", cty.StringVal("Log analytics destination type must be one of: 'Dedicated', 'AzureDiagnostics'."))
	}
	
	// Validation 2: at least one destination must be set
	{
		validation := diagBody.AppendNewBlock("validation", nil)
		validationBody := validation.Body()

		varDS := hclgen.TokensForTraversal("var", "diagnostic_settings")
		orExpr := hclwrite.Tokens{}
		orExpr = append(orExpr, hclgen.TokensForTraversal("v", "workspace_resource_id")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte(" != ")})
		orExpr = append(orExpr, hclwrite.TokensForIdentifier("null")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
		orExpr = append(orExpr, hclgen.TokensForTraversal("v", "storage_account_resource_id")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte(" != ")})
		orExpr = append(orExpr, hclwrite.TokensForIdentifier("null")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
		orExpr = append(orExpr, hclgen.TokensForTraversal("v", "event_hub_authorization_rule_resource_id")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte(" != ")})
		orExpr = append(orExpr, hclwrite.TokensForIdentifier("null")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte(" || ")})
		orExpr = append(orExpr, hclgen.TokensForTraversal("v", "marketplace_partner_resource_id")...)
		orExpr = append(orExpr, &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte(" != ")})
		orExpr = append(orExpr, hclwrite.TokensForIdentifier("null")...)

		listComp := hclwrite.Tokens{
			&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("for")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("_")},
			&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("v")},
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in")},
		}
		listComp = append(listComp, varDS...)
		listComp = append(listComp, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":")})
		listComp = append(listComp, orExpr...)
		listComp = append(listComp, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})

		validationBody.SetAttributeRaw("condition", hclwrite.TokensForFunctionCall("alltrue", listComp))
		validationBody.SetAttributeValue("error_message", cty.StringVal("At least one of `workspace_resource_id`, `storage_account_resource_id`, `marketplace_partner_resource_id`, or `event_hub_authorization_rule_resource_id`, must be set."))
	}
}

// emitPrivateEndpointsVars generates both private_endpoints and private_endpoints_manage_dns_zone_group variables if supported.
func emitPrivateEndpointsVars(body *hclwrite.Body, caps openapi.InterfaceCapabilities, appendVariable func(string, string, hclwrite.Tokens) *hclwrite.Body) {
	if !caps.SupportsPrivateEndpoints {
		return
	}
	
	peBody := appendVariable(
		"private_endpoints",
		"A map of private endpoints to create on this resource.",
		hclwrite.TokensForFunctionCall("map", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("role_assignments"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
				{Name: hclwrite.TokensForIdentifier("role_definition_id_or_name"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("principal_id"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("description"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
				{Name: hclwrite.TokensForIdentifier("skip_service_principal_aad_check"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("bool"), hclwrite.TokensForIdentifier("false"))},
				{Name: hclwrite.TokensForIdentifier("condition"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
				{Name: hclwrite.TokensForIdentifier("condition_version"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
				{Name: hclwrite.TokensForIdentifier("delegated_managed_identity_resource_id"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
				{Name: hclwrite.TokensForIdentifier("principal_type"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			}))), hclwrite.TokensForObject(nil))},
			{Name: hclwrite.TokensForIdentifier("lock"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
				{Name: hclwrite.TokensForIdentifier("kind"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			})), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("tags"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("subnet_resource_id"), Value: hclwrite.TokensForIdentifier("string")},
			{Name: hclwrite.TokensForIdentifier("subresource_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForIdentifier("null"))},
			{Name: hclwrite.TokensForIdentifier("private_dns_zone_group_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"), hclwrite.TokensForValue(cty.StringVal("default")))},
			{Name: hclwrite.TokensForIdentifier("private_dns_zone_resource_ids"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("set", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.ListValEmpty(cty.String)))},
			{Name: hclwrite.TokensForIdentifier("application_security_group_associations"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForIdentifier("string")), hclwrite.TokensForValue(cty.MapValEmpty(cty.String)))},
			{Name: hclwrite.TokensForIdentifier("private_service_connection_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"))},
			{Name: hclwrite.TokensForIdentifier("network_interface_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"))},
			{Name: hclwrite.TokensForIdentifier("location"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"))},
			{Name: hclwrite.TokensForIdentifier("resource_group_name"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForIdentifier("string"))},
			{Name: hclwrite.TokensForIdentifier("ip_configurations"), Value: hclwrite.TokensForFunctionCall("optional", hclwrite.TokensForFunctionCall("map", hclwrite.TokensForFunctionCall("object", hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
				{Name: hclwrite.TokensForIdentifier("name"), Value: hclwrite.TokensForIdentifier("string")},
				{Name: hclwrite.TokensForIdentifier("private_ip_address"), Value: hclwrite.TokensForIdentifier("string")},
			}))), hclwrite.TokensForObject(nil))},
		}))),
	)
	peBody.SetAttributeValue("default", cty.MapValEmpty(cty.DynamicPseudoType))
	peBody.SetAttributeValue("nullable", cty.False)
	body.AppendNewline()

	// private_endpoints_manage_dns_zone_group
	peMgmtBody := appendVariable(
		"private_endpoints_manage_dns_zone_group",
		"Whether to manage private DNS zone groups with this module.",
		hclwrite.TokensForIdentifier("bool"),
	)
	peMgmtBody.SetAttributeValue("default", cty.True)
	peMgmtBody.SetAttributeValue("nullable", cty.False)
}
