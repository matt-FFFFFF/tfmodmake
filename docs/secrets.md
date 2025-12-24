# Secrets Handling in tfmodmake

## Overview

tfmodmake implements comprehensive secrets handling for Azure OpenAPI specifications that mark sensitive fields using the `x-ms-secret` extension. This document explains how secrets are detected, surfaced as ephemeral variables, and integrated with AzAPI's `sensitive_body` and versioning mechanisms.

## What are Secret Fields?

In Azure OpenAPI specifications, sensitive fields like passwords, connection strings, and API keys are marked with the `x-ms-secret: true` extension. These fields require special handling to:

- Prevent accidental exposure in logs and state files
- Support lifecycle management through versioning
- Enable secure passing of sensitive values through Terraform

### Example

```json
{
  "properties": {
    "connectionString": {
      "type": "string",
      "description": "Application Insights connection string",
      "x-ms-secret": true
    },
    "apiKey": {
      "type": "string", 
      "description": "API key for external service",
      "x-ms-secret": true
    }
  }
}
```

## Detection and Collection

### Recursive Schema Traversal

The `collectSecretFields` function traverses the OpenAPI schema recursively to detect all fields marked with `x-ms-secret: true`:

- **Root-level properties**: Direct properties in the schema
- **Nested objects**: Properties within complex object types
- **Array items**: Object properties within array item schemas
- **Deep nesting**: Recursively processes all levels

Example schema structure:
```json
{
  "properties": {
    "daprConfig": {
      "type": "object",
      "properties": {
        "aiConnectionString": {
          "type": "string",
          "x-ms-secret": true
        }
      }
    }
  }
}
```

This would detect the secret at path `properties.daprConfig.aiConnectionString`.

### Path Tracking

Each detected secret includes:
- **path**: JSON path to the field (e.g., `properties.daprAIInstrumentationKey`)
- **varName**: Snake-case Terraform variable name (e.g., `dapr_ai_instrumentation_key`)
- **schema**: The OpenAPI schema for type mapping and validation

## Generated Terraform Artifacts

### 1. Ephemeral Variables

Secret fields are generated as **ephemeral variables** in `variables.tf` with `ephemeral = true`. This leverages Terraform 1.10+ ephemeral values feature to prevent secrets from being persisted in state.

**OpenAPI:**
```json
{
  "connectionString": {
    "type": "string",
    "description": "Application Insights connection string",
    "x-ms-secret": true
  }
}
```

**Generated variables.tf:**
```hcl
variable "connection_string" {
  description = "Application Insights connection string"
  type        = string
  default     = null
  ephemeral   = true
}

variable "connection_string_version" {
  description = "Version tracker for connection_string. Must be set when connection_string is provided."
  type        = number
  default     = null
  
  validation {
    condition     = var.connection_string == null || var.connection_string_version != null
    error_message = "When connection_string is set, connection_string_version must also be set."
  }
}
```

### 2. Version Tracking Variables

For each secret variable, a corresponding `<secret_name>_version` variable is generated:

- **Purpose**: Enables lifecycle management by forcing resource updates when secrets rotate
- **Type**: `number` 
- **Validation**: Enforces that the version must be set when the secret is provided
- **Usage**: Users increment this value when updating the secret

**Example usage:**
```hcl
module "container_app_env" {
  source = "./modules/container-app-env"
  
  connection_string         = var.app_insights_connection_string
  connection_string_version = 1  # Increment to 2 when rotating the secret
}
```

### 3. Exclusion from Regular Body

Secret fields are **excluded** from the regular `body` attribute in `main.tf`. They do not appear in `locals.tf` to prevent inclusion in the standard resource body.

**Generated locals.tf:**
```hcl
locals {
  resource_body = {
    properties = {
      # Non-secret fields only
      normalField = var.normal_field
      # connectionString is NOT here
    }
  }
}
```

### 4. AzAPI sensitive_body Integration

Secrets are passed through the `azapi_resource.sensitive_body` attribute, which:
- Accepts a nested object structure matching the API schema
- Marks the entire attribute as sensitive
- Deep-merges with the regular `body` at apply time

**Generated main.tf:**
```hcl
resource "azapi_resource" "this" {
  type      = "Microsoft.App/managedEnvironments@2024-03-01"
  name      = var.name
  parent_id = var.parent_id
  
  body = local.resource_body
  
  sensitive_body = {
    properties = {
      daprAIInstrumentationKey = var.dapr_ai_instrumentation_key
      daprAIConnectionString   = var.dapr_ai_connection_string
    }
  }
  
  sensitive_body_version = {
    "properties.daprAIInstrumentationKey" = var.dapr_ai_instrumentation_key_version
    "properties.daprAIConnectionString"   = var.dapr_ai_connection_string_version
  }
}
```

### 5. Version Map Generation

The `sensitive_body_version` attribute maps each secret's JSON path to its version variable:

```hcl
sensitive_body_version = {
  "properties.connectionString" = var.connection_string_version
  "properties.apiKey"           = var.api_key_version
}
```

This enables AzAPI to track version changes and force updates when secrets rotate.

## Implementation Details

### Tree-Based sensitive_body Construction

The `tokensForSensitiveBody` function builds the `sensitive_body` object hierarchically:

1. **Parse paths**: Split each secret path into segments (e.g., `properties.daprConfig.apiKey`)
2. **Build tree**: Construct a tree structure matching the JSON hierarchy
3. **Render recursively**: Generate nested HCL object notation from the tree

This ensures the `sensitive_body` structure matches the API schema exactly.

**Example:**
```
Paths:
  - properties.config.apiKey
  - properties.config.secret
  - properties.otherSecret

Tree:
  properties
    ├── config
    │   ├── apiKey
    │   └── secret
    └── otherSecret

Generated:
  {
    properties = {
      config = {
        apiKey = var.api_key
        secret = var.secret
      }
      otherSecret = var.other_secret
    }
  }
```

### Flattened vs Nested Secrets

Secrets are **always surfaced as top-level variables**, even when deeply nested in the schema:

**Schema:**
```json
{
  "properties": {
    "daprConfig": {
      "type": "object",
      "properties": {
        "aiConnectionString": {
          "x-ms-secret": true
        }
      }
    }
  }
}
```

**Generated variables** (not nested):
```hcl
variable "ai_connection_string" {
  ephemeral = true
  # ...
}
```

**Mapped in sensitive_body** (nested):
```hcl
sensitive_body = {
  properties = {
    daprConfig = {
      aiConnectionString = var.ai_connection_string
    }
  }
}
```

### Collision Detection

The implementation prevents variable name collisions between:
- Regular properties flattened from the `properties` bag
- Secret fields extracted from nested structures
- Base variables (`name`, `parent_id`, `location`, `tags`)

If a collision is detected, generation fails with a clear error message.

## Terraform 1.10+ Ephemeral Values

### What are Ephemeral Values?

Ephemeral values are a Terraform 1.10+ feature that:
- Exist only during the plan/apply lifecycle
- Are **never** persisted to state files
- Cannot be used in outputs or module outputs
- Are ideal for sensitive data like passwords and API keys

### Usage Requirements

**Terraform version:**
```hcl
terraform {
  required_version = ">= 1.10"
}
```

**Provider support:**
The AzAPI provider must support ephemeral inputs for `sensitive_body`. Check provider version compatibility.

### Limitations

1. **Cannot output ephemeral values**: You cannot create outputs from ephemeral variables
2. **Cannot persist**: Ephemeral values don't survive Terraform refresh operations
3. **Must re-provide**: Secrets must be provided on every plan/apply operation
4. **No defaults from state**: Unlike regular variables, ephemeral variables cannot have their values read from state

### Best Practices

1. **Use version tracking**: Always set the `_version` variable when updating secrets
2. **Rotate regularly**: Increment versions to force resource updates during rotation
3. **Store externally**: Pull secrets from external secret stores (Key Vault, AWS Secrets Manager)
4. **Document rotation**: Include rotation procedures in module documentation

## Real-World Example: Azure Container App Environment

### OpenAPI Specification

```json
{
  "Microsoft.App/managedEnvironments": {
    "properties": {
      "daprAIInstrumentationKey": {
        "type": "string",
        "description": "Application Insights instrumentation key for Dapr",
        "x-ms-secret": true
      },
      "daprAIConnectionString": {
        "type": "string", 
        "description": "Application Insights connection string for Dapr",
        "x-ms-secret": true
      },
      "workloadProfiles": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "name": {"type": "string"}
          }
        }
      }
    }
  }
}
```

### Generated Code

**variables.tf:**
```hcl
variable "name" {
  description = "The name of the resource."
  type        = string
}

variable "parent_id" {
  description = "The parent resource ID for this resource."
  type        = string
}

variable "location" {
  description = "The location of the resource."
  type        = string
}

variable "workload_profiles" {
  description = "The workload_profiles of the resource."
  type = list(object({
    name = optional(string)
  }))
  default = null
}

variable "dapr_ai_instrumentation_key" {
  description = "Application Insights instrumentation key for Dapr"
  type        = string
  default     = null
  ephemeral   = true
}

variable "dapr_ai_instrumentation_key_version" {
  description = "Version tracker for dapr_ai_instrumentation_key. Must be set when dapr_ai_instrumentation_key is provided."
  type        = number
  default     = null
  
  validation {
    condition     = var.dapr_ai_instrumentation_key == null || var.dapr_ai_instrumentation_key_version != null
    error_message = "When dapr_ai_instrumentation_key is set, dapr_ai_instrumentation_key_version must also be set."
  }
}

variable "dapr_ai_connection_string" {
  description = "Application Insights connection string for Dapr"
  type        = string
  default     = null
  ephemeral   = true
}

variable "dapr_ai_connection_string_version" {
  description = "Version tracker for dapr_ai_connection_string. Must be set when dapr_ai_connection_string is provided."
  type        = number
  default     = null
  
  validation {
    condition     = var.dapr_ai_connection_string == null || var.dapr_ai_connection_string_version != null
    error_message = "When dapr_ai_connection_string is set, dapr_ai_connection_string_version must also be set."
  }
}
```

**locals.tf:**
```hcl
locals {
  resource_body = {
    properties = {
      workloadProfiles = var.workload_profiles
      # Secrets are NOT included here
    }
  }
}
```

**main.tf:**
```hcl
resource "azapi_resource" "this" {
  type                  = "Microsoft.App/managedEnvironments@2024-03-01"
  name                  = var.name
  parent_id             = var.parent_id
  location              = var.location
  ignore_null_property  = true
  
  body = local.resource_body
  
  sensitive_body = {
    properties = {
      daprAIConnectionString   = var.dapr_ai_connection_string
      daprAIInstrumentationKey = var.dapr_ai_instrumentation_key
    }
  }
  
  sensitive_body_version = {
    "properties.daprAIConnectionString"   = var.dapr_ai_connection_string_version
    "properties.daprAIInstrumentationKey" = var.dapr_ai_instrumentation_key_version
  }
  
  response_export_values = []
}
```

### Module Usage

```hcl
module "container_app_env" {
  source = "./modules/container-app-env"
  
  name      = "my-env"
  parent_id = azurerm_resource_group.example.id
  location  = "eastus"
  
  workload_profiles = [
    {
      name = "Consumption"
    }
  ]
  
  # Secrets from Key Vault
  dapr_ai_connection_string         = data.azurerm_key_vault_secret.app_insights_cs.value
  dapr_ai_connection_string_version = 1
  
  dapr_ai_instrumentation_key         = data.azurerm_key_vault_secret.app_insights_key.value
  dapr_ai_instrumentation_key_version = 1
}
```

## Testing

The secrets handling implementation includes comprehensive tests in `internal/terraform/generator_test.go`:

### TestGenerate_WithSecretFields

Tests that:
- ✅ Secret fields are marked `ephemeral = true`
- ✅ Version variables are generated with validations
- ✅ Secrets are excluded from `locals.tf` body
- ✅ `sensitive_body` attribute contains all secrets
- ✅ `sensitive_body_version` maps paths to version variables
- ✅ No variable name collisions occur
- ✅ Flattened properties don't create duplicate secret variables

### TestGenerate_WithNestedSecrets

Tests that:
- ✅ Deeply nested secrets are detected
- ✅ Correct JSON paths are generated
- ✅ Tree-based `sensitive_body` structure is correct
- ✅ Array item secrets are handled properly

### TestIsSecretField

Tests that:
- ✅ Fields with `x-ms-secret: true` are detected
- ✅ Fields without the extension are ignored
- ✅ Invalid extension values (non-boolean) are handled
- ✅ Nil schemas don't cause panics

## Integration with Other Features

### allOf Handling

Secrets detection works seamlessly with `allOf` schema composition:
- Schemas are flattened before secret collection
- Secrets in base types are detected
- Merged properties are checked for `x-ms-secret`

### Validations

Secret variables receive the same validation generation as regular variables:
- Type-based validations (minLength, maxLength, etc.)
- Enum validations
- Plus the required version validation

### Read-Only Properties

Fields marked as both `readOnly: true` and `x-ms-secret: true` are ignored (they cannot be set by users).

## Error Handling

### Variable Name Collisions

```
Error: terraform variable name collision: "api_key" (from properties.apiKey)
```

Occurs when a secret variable name conflicts with an existing variable. Resolution: manually rename one of the properties or use `-root` flag to scope generation.

### Missing Version When Secret is Set

The validation block enforces this at plan time:

```
Error: When connection_string is set, connection_string_version must also be set.
```

Resolution: Always provide the version variable when providing a secret.

### Invalid x-ms-secret Value

Non-boolean values for `x-ms-secret` are silently ignored (treated as false). This prevents generation errors for malformed specs.

## Future Enhancements

Potential improvements:
1. **Auto-increment versions**: Optional feature to auto-increment version from previous state
2. **Secret references**: Support for direct Key Vault references in generated code
3. **Rotation helpers**: Generate helper scripts for secret rotation workflows
4. **Audit logging**: Track when secrets are updated in Terraform operations
5. **Multiple secret sources**: Support mixing ephemeral inputs with references

## Design Rationale

### Why Ephemeral Variables?

Ephemeral variables prevent secrets from being persisted to state, addressing a long-standing security concern in Terraform. This is superior to marking outputs as sensitive, which only redacts display but still stores values in state.

### Why Separate Version Variables?

Azure resources often require explicit updates when secrets rotate. Version tracking enables:
- Deterministic updates (increment version → force replacement)
- Audit trail of when secrets were rotated
- Prevention of drift when secrets change externally

### Why Flatten Secret Variables?

Flattening secrets to top-level variables (even when nested in schema) provides:
- Consistent user experience
- Easier integration with secret management tools
- Simpler variable passing between modules
- Clear visibility of all required secrets

### Why Tree-Based sensitive_body?

Building the `sensitive_body` as a tree structure:
- Ensures exact API schema compliance
- Supports arbitrary nesting levels
- Handles complex schemas with multiple secret layers
- Maintains readability in generated code

## Performance Considerations

Secret detection is performed during generation (not runtime):
- One-time schema traversal cost
- Cached results for repeated references
- No runtime overhead in Terraform operations
- Fast path for schemas without secrets (early return)

The tree-based rendering is O(n) where n is the number of secrets, with minimal allocation overhead.
