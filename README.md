# tfmodmake

CLI tool to generate base Terraform configuration (`variables.tf` and `locals.tf`) from an OpenAPI specification.

## Features

*   Parses OpenAPI 3.0 specifications from local files or URLs.
*   Extracts schema for a specific resource type.
*   Generates Terraform variables with appropriate types and descriptions.
*   Flattens the OpenAPI top-level `properties` bag into idiomatic top-level Terraform variables.
*   Handles nested objects and arrays.
*   **Generates comprehensive validation blocks from schema constraints:**
    *   String validations: `minLength`, `maxLength`, `pattern`, `format` (UUID)
    *   Array validations: `minItems`, `maxItems`, `uniqueItems`
    *   Numeric validations: `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`
    *   Enum validations: direct `enum`, `allOf`, Azure `x-ms-enum`
    *   All validations are null-safe for optional fields
*   Creates a `locals.tf` file to map Terraform variables back to the API JSON structure.
*   Supports targeting a specific root object (e.g., `properties`) to exclude unwanted fields.
*   Customizable local variable naming.
*   Generates scaffolded `main.tf` and `outputs.tf` for an `azapi_resource`.
*   Includes base variables for `name`, `parent_id`, and conditional `tags` (when the resource supports tags).
*   **Generates `response_export_values` from computed (non-writable) fields in the schema:**
    *   Automatically extracts computed/non-writable properties (including scalars and useful objects/arrays)
    *   Applies filtering to remove noisy fields (array indices, `.status.`, `.provisioningError.`, `eTag`, timestamps)
    *   Provides a useful starting point that module authors can trim to their needs
*   Generates map-based module blocks for submodules using `addsub` command.
*   **Discovers deployable child resources from OpenAPI specs using `children` command:**
    *   Identifies ARM child resource types under a parent resource
    *   Filters resources by deployability (PUT/PATCH with request body)
    *   Supports multiple specs with API version preference
    *   Outputs markdown or JSON format

## Installation

Build from source:

```bash
git clone https://github.com/user/tfmodmake.git
cd tfmodmake
go build -o tfmodmake cmd/tfmodmake/main.go
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

### Child Resource Discovery

See also: docs/children-discovery.md

To discover deployable child resources under a parent resource type:

```bash
./tfmodmake children -spec <path_or_url> [-discover] [-discover-latest] [-discover-include-preview] [-discover-prefer-preview] [-include <glob>] [-github-dir <url>] -parent <resource_type> [-json]
```

This command inspects OpenAPI specs and returns child resource types that can be deployed under a parent resource.

**Flags:**

*   `-spec`: (Required, repeatable) Path or URL to OpenAPI specification. Can be specified multiple times to search across versions.
*   `-discover`: (Optional) If `-spec` is a `raw.githubusercontent.com` URL, discover additional spec files from the same GitHub directory (via the GitHub contents API) and include them automatically.
*   `-discover-latest`: (Optional) When discovering from GitHub (`-discover` and/or `-github-dir`), select the latest API version folder deterministically (stable by default).
*   `-discover-include-preview`: (Optional) When used with `-discover-latest`, also include specs from the latest preview API version folder.
*   `-discover-prefer-preview`: (Optional) When used with `-discover-latest`, prefer preview over stable (useful when there is no stable version yet).
*   `-github-dir`: (Optional) GitHub directory URL in the form `https://github.com/<owner>/<repo>/tree/<ref>/<dir>` to discover spec files from (via the GitHub contents API).
*   `-include`: (Optional) Glob matched against filenames when discovering spec files (default `*.json`). If you leave it as the default, `children` will try a narrower `ParentName*.json` pattern first (e.g. `ManagedEnvironments*.json`), and fall back to `*.json` if nothing matches.
*   `-parent`: (Required) Parent resource type (e.g., `Microsoft.App/managedEnvironments`).
*   `-json`: (Optional) Output results as JSON instead of plain text.

**Example:**

```bash
./tfmodmake children \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json" \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironmentsDaprComponents.json" \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironmentsStorages.json" \
  -parent "Microsoft.App/managedEnvironments"
```

To avoid listing all the spec files manually, you can discover siblings from a single raw GitHub spec URL:

```bash
./tfmodmake children \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/app/resource-manager/Microsoft.App/ContainerApps/preview/2025-10-02-preview/ManagedEnvironments.json" \
  -discover \
  -include "ManagedEnvironments*.json" \
  -parent "Microsoft.App/managedEnvironments"
```

To make discovery more deterministic when you only have a version folder (or you want a stable-by-default starting point), use `-discover-latest`:

```bash
./tfmodmake children \
  -github-dir "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps" \
  -discover-latest \
  -discover-include-preview \
  -parent "Microsoft.App/managedEnvironments"
```

Output shows:
*   **Deployable Child Resources**: Resources with PUT/PATCH operations and request body schemas
*   **Filtered Out**: Resources that cannot be deployed (GET-only, missing body schema, etc.) with reasons

Note: the default output is intentionally plain and compact for terminal use. Use `-json` if you want structured output (including example paths) for scripting or deeper inspection.

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
