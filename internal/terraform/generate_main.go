package terraform

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/internal/hclgen"
	"github.com/zclconf/go-cty/cty"
)

func cleanTypeString(typeStr string) string {
	segments := strings.Split(typeStr, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			continue
		}
		cleaned = append(cleaned, segment)
	}
	return strings.Join(cleaned, "/")
}

func generateMain(schema *openapi3.Schema, resourceType, apiVersion, localName string, supportsTags, supportsLocation, supportsIdentity, hasSchema bool, secrets []secretField) error {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		apiVersion = "apiVersion"
	}
	resourceTypeWithAPIVersion := fmt.Sprintf("%s@%s", cleanTypeString(resourceType), apiVersion)

	resourceBlock := body.AppendNewBlock("resource", []string{"azapi_resource", "this"})
	resourceBody := resourceBlock.Body()
	resourceBody.SetAttributeValue("type", cty.StringVal(resourceTypeWithAPIVersion))
	resourceBody.SetAttributeRaw("name", hclgen.TokensForTraversal("var", "name"))
	resourceBody.SetAttributeRaw("parent_id", hclgen.TokensForTraversal("var", "parent_id"))
	resourceBody.SetAttributeValue("ignore_null_property", cty.BoolVal(true))
	resourceBody.SetAttributeValue("schema_validation_enabled", cty.BoolVal(true))

	if supportsLocation {
		resourceBody.SetAttributeRaw("location", hclgen.TokensForTraversal("var", "location"))
	}

	resourceBody.SetAttributeValue("body", cty.EmptyObjectVal)
	if hasSchema {
		// If the schema already has a top-level "properties" field, we are generating the full resource body.
		// Otherwise, we assume the schema itself represents the properties object (e.g. -root properties).
		_, hasTopLevelProperties := schema.Properties["properties"]
		if hasTopLevelProperties {
			resourceBody.SetAttributeRaw("body", hclgen.TokensForTraversal("local", localName))
		} else {
			resourceBody.SetAttributeRaw("body", hclwrite.TokensForObject(
				[]hclwrite.ObjectAttrTokens{
					{
						Name:  hclwrite.TokensForIdentifier("properties"),
						Value: hclgen.TokensForTraversal("local", localName),
					},
				},
			))
		}
	}

	// Add sensitive_body if there are secrets
	if len(secrets) > 0 {
		resourceBody.SetAttributeRaw("sensitive_body", tokensForSensitiveBody(secrets, func(secret secretField) hclwrite.Tokens {
			return hclgen.TokensForTraversal("var", secret.varName)
		}))

		// Add sensitive_body_version map
		var versionAttrs []hclwrite.ObjectAttrTokens
		for _, secret := range secrets {
			versionVarName := secret.varName + "_version"
			versionAttrs = append(versionAttrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForValue(cty.StringVal(secret.path)),
				Value: hclgen.TokensForTraversal("var", versionVarName),
			})
		}
		resourceBody.SetAttributeRaw("sensitive_body_version", hclwrite.TokensForObject(versionAttrs))
	}

	if supportsTags {
		resourceBody.SetAttributeRaw("tags", hclgen.TokensForTraversal("var", "tags"))
	}

	if supportsIdentity {
		dyn := resourceBody.AppendNewBlock("dynamic", []string{"identity"})
		dynBody := dyn.Body()
		dynBody.SetAttributeRaw("for_each", hclgen.TokensForTraversal("local", "managed_identities", "system_assigned_user_assigned"))

		content := dynBody.AppendNewBlock("content", nil)
		contentBody := content.Body()
		contentBody.SetAttributeRaw("type", hclgen.TokensForTraversal("identity", "value", "type"))
		contentBody.SetAttributeRaw("identity_ids", hclgen.TokensForTraversal("identity", "value", "user_assigned_resource_ids"))
	}

	// Generate response_export_values from computed (non-writable) fields in the schema
	exportPaths := extractComputedPaths(schema)
	if len(exportPaths) > 0 {
		resourceBody.SetAttributeRaw("response_export_values", hclgen.TokensForMultilineStringList(exportPaths))

		// Add a reminder comment after the resource block.
		// This placement makes it stand out to users who should customize the exports.
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenComment, Bytes: []byte("\n# Trim response_export_values to only the computed fields you need.\n")},
		})
	} else {
		resourceBody.SetAttributeValue("response_export_values", cty.ListValEmpty(cty.String))
	}

	return hclgen.WriteFile("main.tf", file)
}
