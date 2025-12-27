# tfmodmake

CLI tool to generate base Terraform configuration from an OpenAPI specification.

## Features

*   **OpenAPI → Terraform scaffolding**: Generate `variables.tf`, `locals.tf`, `main.tf`, and `outputs.tf` for an `azapi_resource`.
*   **Good Terraform ergonomics**: Strong typing, descriptions, nested object/array handling, and optional “flatten `properties`” variable shape.
*   **Schema-driven validations**: Null-safe validation blocks from common constraints (lengths, patterns, ranges, enums).
*   **Computed exports**: Auto-suggest `response_export_values` from read-only/non-writable response fields (with noise filtering).
*   **Submodule helpers**: `addsub` generates map-based wrapper plumbing for submodules.
*   **Scope discovery**: `children` lists deployable ARM child resource types under a parent (compact text or `-json`).
*   **Child module composition**: `addchild` orchestrates end-to-end child module generation and wiring.

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


### Child Module Generation and Wiring

The `addchild` command orchestrates the complete process of creating a child module and wiring it into the parent module:

```bash
./tfmodmake addchild -parent <parent_type> -child <child_type> [flags]
```

**Required flags:**

*   `-parent`: Parent resource type (e.g., `Microsoft.App/managedEnvironments`)
*   `-child`: Child resource type (e.g., `Microsoft.App/managedEnvironments/storages`)

**Spec selection (one of):**

*   `-spec-root`: (Recommended) GitHub tree URL under `Azure/azure-rest-api-specs` pointing at the service root directory. Automatically selects latest stable API version.
*   `-spec`: Explicit spec path or URL (can be specified multiple times)

**Optional flags:**

*   `-include-preview`: Also include latest preview API version (only with `-spec-root`)
*   `-include`: Glob pattern to filter spec files (default: `*.json`)
*   `-module-dir`: Directory for child modules (default: `modules`)
*   `-module-name`: Override derived module folder name (default: derived from child type). **Recommended:** use singular form (e.g., `-module-name storage` instead of auto-derived `storages`) to follow the convention that each submodule manages one resource instance.
*   `-dry-run`: Print planned actions without writing files

**What it does:**

1.  Generates a complete child module scaffold at `<module-dir>/<module-name>/`
2.  Wires the child module into the root module using the same mechanics as `addsub`

**Module naming convention:**

By default, the module name is derived from the last segment of the child resource type (e.g., `.../storages` → `storages`). However, following the project convention that each submodule makes one thing (singular), it's recommended to use `-module-name` to specify a singular name when the derived name is plural.

**Example:**

```bash
# Using spec-root (recommended) with singular module name
./tfmodmake addchild \
  -parent "Microsoft.App/managedEnvironments" \
  -child "Microsoft.App/managedEnvironments/storages" \
  -module-name "storage" \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps"

# Using explicit spec with singular module name
./tfmodmake addchild \
  -parent "Microsoft.KeyVault/vaults" \
  -child "Microsoft.KeyVault/vaults/secrets" \
  -module-name "secret" \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2024-11-01/secrets.json"

# Custom module directory and name
./tfmodmake addchild \
  -parent "Microsoft.App/managedEnvironments" \
  -child "Microsoft.App/managedEnvironments/certificates" \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps" \
  -module-dir "submodules" \
  -module-name "certificate"
```

**Generated files:**

*   `<module-dir>/<module-name>/variables.tf`: Child module variables
*   `<module-dir>/<module-name>/locals.tf`: Child module locals
*   `<module-dir>/<module-name>/main.tf`: Child module resource
*   `<module-dir>/<module-name>/outputs.tf`: Child module outputs
*   `variables.<module-name>.tf`: Root module variable for child instances
*   `main.<module-name>.tf`: Root module wrapper with `for_each`


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

See also: [docs/children-discovery.md](docs/children-discovery.md)

The `children` command inspects OpenAPI specs and returns child resource types that can be deployed under a parent resource.

```bash
./tfmodmake children -spec <path_or_url> -parent <resource_type> [-json]
```

**Common flags:**

*   `-spec`: (Required, repeatable) Path or URL to OpenAPI specification. Can be specified multiple times to search across versions.
*   `-parent`: (Required) Parent resource type (e.g., `Microsoft.App/managedEnvironments`).
*   `-json`: (Optional) Output results as JSON instead of plain text.

**Discovery flags (advanced):**

Recommended: use `-spec-root`.

It gives you a deterministic “latest stable” starting point without manually enumerating spec URLs.

Example:

```bash
./tfmodmake children \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps" \
  -include-preview \
  -parent "Microsoft.App/managedEnvironments"
```

Other discovery options (details in [docs/children-discovery.md](docs/children-discovery.md)):

- `-discover`: when `-spec` is a `raw.githubusercontent.com` URL, pull in sibling spec files from the same directory.
- `-include`: restrict which spec files are included during discovery (glob).
- If you hit GitHub rate limits, set `GITHUB_TOKEN` (or `GH_TOKEN`) and retry.

**Debugging:**

*   `-print-resolved-specs`: (Optional) Print the final resolved spec list to **stderr** before analysis. Useful for diagnosing missing children without polluting stdout/JSON output.

Output shows:
*   **Deployable Child Resources**: Resources with PUT/PATCH operations and request body schemas
*   **Filtered Out**: Resources that cannot be deployed (GET-only, missing body schema, etc.) with reasons

Note: the default output is intentionally plain and compact for terminal use. Use `-json` if you want structured output (including example paths) for scripting or deeper inspection.
