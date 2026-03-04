package terraform

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/matt-FFFFFF/tfmodmake/hclgen"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/schema"
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

func buildMain(rs *schema.ResourceSchema, resourceType, apiVersion, localName string, supportsTags, supportsLocation, supportsIdentity, hasSchema, hasDiscriminator bool, secrets []secretField) *hclwrite.File {
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

	if supportsLocation {
		resourceBody.SetAttributeRaw("location", hclgen.TokensForTraversal("var", "location"))
	}

	resourceBody.SetAttributeValue("body", cty.EmptyObjectVal)
	if hasSchema {
		resourceBody.SetAttributeRaw("body", hclgen.TokensForTraversal("local", localName))
	}

	// Disable embedded schema validation for resources whose body contains a
	// discriminated object type (e.g. javaComponents with componentType).
	// The azapi provider performs enum validation on discriminator properties at
	// plan/validate time, but Terraform passes "unknown" for unset variables
	// which the provider rejects as an invalid discriminator value.
	// TODO: re-enable once the azapi provider handles unknown discriminator values gracefully.
	if hasDiscriminator {
		resourceBody.AppendUnstructuredTokens(hclwrite.Tokens{
			&hclwrite.Token{Type: hclsyntax.TokenComment, Bytes: []byte("# Disabled because the body contains a discriminated object type whose")},
			&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			&hclwrite.Token{Type: hclsyntax.TokenComment, Bytes: []byte("# discriminator property value is unknown at validate time.")},
			&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		})
		resourceBody.SetAttributeValue("schema_validation_enabled", cty.False)
	}

	// Add sensitive_body if there are secrets
	if len(secrets) > 0 {
		// Null-check intermediate containers in sensitive_body that correspond
		// to flattened property variables. Without this, AzAPI schema validation
		// rejects partial objects that are missing required non-secret siblings
		// (e.g., servicePrincipalProfile.clientId when only .secret is present).
		sensitiveNullCheck := func(pathSegments []string) hclwrite.Tokens {
			if len(pathSegments) != 2 || pathSegments[0] != "properties" {
				return nil
			}
			varName := naming.ToSnakeCase(pathSegments[1])
			return hclgen.TokensForTraversal("var", varName)
		}

		sensitiveBodyTokens := tokensForSensitiveBody(secrets, func(secret secretField) hclwrite.Tokens {
			return hclgen.TokensForTraversal("var", secret.varName)
		}, sensitiveNullCheck)
		resourceBody.SetAttributeRaw("sensitive_body", sensitiveBodyTokens)

		// Add sensitive_body_version map
		var versionAttrs []hclwrite.ObjectAttrTokens
		for _, secret := range secrets {
			versionVarName := secret.varName + "_version"
			key := secret.path
			versionAttrs = append(versionAttrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForValue(cty.StringVal(key)),
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
	exportPaths := extractComputedPaths(rs)
	resourceBody.SetAttributeRaw("response_export_values", hclgen.TokensForMultilineStringList(exportPaths))

	return file
}

func generateMain(rs *schema.ResourceSchema, resourceType, apiVersion, localName string, supportsTags, supportsLocation, supportsIdentity, hasSchema, hasDiscriminator bool, secrets []secretField, outputDir string) error {
	return hclgen.WriteFileToDir(outputDir, "main.tf", buildMain(rs, resourceType, apiVersion, localName, supportsTags, supportsLocation, supportsIdentity, hasSchema, hasDiscriminator, secrets))
}
