# tfmodmake

CLI tool to generate a base Terraform module from an OpenAPI specification.

## Features

*   **OpenAPI → Terraform scaffolding**: Generate `variables.tf`, `locals.tf`, `main.tf`, and `outputs.tf` for an `azapi_resource`.
*   **Good Terraform ergonomics**: Strong typing, descriptions, nested object/array handling, and optional “flatten `properties`” variable shape.
*   **Schema-driven validations**: Null-safe validation blocks from common constraints (lengths, patterns, ranges, enums).
*   **Computed exports**: Auto-suggest `response_export_values` from read-only/non-writable response fields (with noise filtering).
*   **Submodule helpers**: `addsub` generates map-based wrapper plumbing for submodules.
*   **Scope discovery**: `children` lists deployable ARM child resource types under a parent (compact text or `-json`).
*   **AVM interfaces scaffolding**: Generate `main.interfaces.tf` wiring for common AVM interfaces (role assignments, locks, diagnostic settings, private endpoints, telemetry).

## Installation

Build from source:

```bash
git clone https://github.com/matt-FFFFFF/tfmodmake.git
cd tfmodmake
go build -o tfmodmake ./cmd/tfmodmake
```

## Usage

```bash
./tfmodmake -spec <path_or_url> -resource <resource_type> [flags]
```

### Flags

*   `-spec`: (Required) Path or URL to the OpenAPI specification.
*   `-resource`: (Required) Resource type to generate configuration for (e.g., `Microsoft.ContainerService/managedClusters`).
*   `-root`: (Optional) Dot-separated path to the root object within the resource schema (e.g., `properties` or `properties.networkProfile`). If specified, only properties under this root are generated as variables.
*   `-local-name`: (Optional) Name of the local variable to generate in `locals.tf`. Defaults to `resource_body` or a snake_case version of the `-root` path.

### Submodule Generation

To generate a map-based module block for a submodule:

```bash
./tfmodmake addsub <path_to_submodule>
```

This command reads the Terraform module at the specified path and generates:
1.  `variables.<module_name>.tf`: A variable accepting a map of objects matching the submodule's inputs.
2.  `main.<module_name>.tf`: A `module` block using `for_each` to iterate over the variable.

## Examples

### Basic Usage

Generate configuration for the entire resource:

```bash
# example with AKS and stable API
./tfmodmake \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/containerservice/resource-manager/Microsoft.ContainerService/aks/stable/2025-10-01/managedClusters.json \
  -resource Microsoft.ContainerService/managedClusters
```

```bash
# example with Container Apps Managed Environment & preview API
./tfmodmake \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json \
  -resource Microsoft.App/managedEnvironments
```

```bash
# KeyVault and KeyVault secret
./tfmodmake \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2025-05-01/openapi.json \
  -resource Microsoft.KeyVault/vaults

./tfmodmake \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2024-11-01/secrets.json \
  -resource Microsoft.KeyVault/vaults/secrets \
  -local-name secret_body
```

### Submodule Wrapper Generation

Generate a map-based wrapper for an existing Terraform submodule:

```bash
# From the parent module directory:
./tfmodmake addsub modules/secrets

# Generates:
# - variables.secrets.tf
# - main.secrets.tf
```

### Targeting a Sub-property

Generate configuration only for the `properties` object, excluding top-level fields like `tags` or `location`:

```bash
./tfmodmake \
  -spec managedClusters.json \
  -resource Microsoft.ContainerService/managedClusters \
  -root properties
```

This will generate a local variable named `properties`.

### Custom Local Name

Generate configuration for `properties.networkProfile` and name the local variable `aks_network_profile`:

```bash
./tfmodmake \
  -spec managedClusters.json \
  -resource Microsoft.ContainerService/managedClusters \
  -root properties.networkProfile \
  -local-name aks_network_profile
```

## Output

The tool generates four files in the current directory:

1.  `variables.tf`: Contains the input variables (including `name`, `parent_id`, and `tags` when supported).
2.  `locals.tf`: Contains the local value constructing the JSON body structure.
3.  `main.tf`: Scaffold for the `azapi_resource` using the generated locals.
4.  `outputs.tf`: Outputs exposing the resource ID and name.

When generating the full resource schema (no `-root`), the OpenAPI top-level `properties` object is flattened so its children become top-level Terraform variables (for example `app_logs_configuration`, `custom_domain_configuration`, etc.), and `locals.tf` reconstructs the JSON `properties` object from those variables.

When using `-root properties`, `locals.tf` represents the `properties` object and `main.tf` wraps it under `body.properties`.

## Validation Blocks

The tool automatically generates Terraform validation blocks from OpenAPI schema constraints, helping catch invalid inputs early with clear error messages. Supported constraints include:

- **String validations**: minLength, maxLength, pattern (regex), format (UUID)
- **Array validations**: minItems, maxItems, uniqueItems
- **Numeric validations**: minimum, maximum, exclusiveMinimum, exclusiveMaximum, multipleOf
- **Enum validations**: Direct enum, allOf composition, Azure x-ms-enum extension

All validations are null-safe for optional fields. See [docs/validations.md](docs/validations.md) for detailed documentation and examples.

## Advanced: Child Resource Discovery

The `children` command inspects OpenAPI specs and returns child resource types that can be deployed under a parent resource.

This is a discovery process that does not make any terraform code, it is designed to be used with the submodule subcommand.

```bash
./tfmodmake children -spec <path_or_url> -parent <resource_type> [-json]
```

**Common flags:**

*   `-spec-root`: (Required, repeatable) Path to OpenAPI specification, see below
*   `-parent`: (Required) Parent resource type (e.g., `Microsoft.App/managedEnvironments`).
*   `-json`: (Optional) Output results as JSON instead of plain text.
*   `-include-preview`: (Optional) Search for preview versions of resources.

`Spec-root` points to the resource manager specification URL, allowing it to enumerate available versions. 

An example:

```bash
./tfmodmake children \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps" \
  -include-preview \
  -parent "Microsoft.App/managedEnvironments"
```

Example output:

```text
Deployable child resources
- 2025-10-02-preview    Microsoft.App/managedEnvironments/certificates
- 2025-10-02-preview    Microsoft.App/managedEnvironments/daprComponents
- 2025-10-02-preview    Microsoft.App/managedEnvironments/daprSubscriptions
- 2025-10-02-preview    Microsoft.App/managedEnvironments/httpRouteConfigs
- 2025-10-02-preview    Microsoft.App/managedEnvironments/maintenanceConfigurations
- 2025-10-02-preview    Microsoft.App/managedEnvironments/managedCertificates
- 2025-10-02-preview    Microsoft.App/managedEnvironments/privateEndpointConnections
- 2025-10-02-preview    Microsoft.App/managedEnvironments/storages

Filtered out
(none)
```

Other discovery options (details in [docs/children-discovery.md](docs/children-discovery.md)):
