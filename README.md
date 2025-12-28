# tfmodmake

CLI tool to generate a base Terraform module from an OpenAPI specification.

## Features

*   **OpenAPI → Terraform scaffolding**: Generate `variables.tf`, `locals.tf`, `main.tf`, and `outputs.tf` for an `azapi_resource`.
*   **Good Terraform ergonomics**: Strong typing, descriptions, nested object/array handling, and optional “flatten `properties`” variable shape.
*   **Schema-driven validations**: Null-safe validation blocks from common constraints (lengths, patterns, ranges, enums).
*   **Computed exports**: Auto-suggest `response_export_values` from read-only/non-writable response fields (with noise filtering).
*   **Submodule helpers**: `add submodule` generates map-based wrapper plumbing for submodules (legacy alias: `addsub`).
*   **Scope discovery**: `discover children` lists deployable ARM child resource types under a parent (compact text or `-json`; legacy alias: `children`).
*   **AVM interfaces scaffolding** (opt-in): Use `add avm-interfaces` to generate `main.interfaces.tf` wiring for common AVM interfaces (role assignments, locks, diagnostic settings, private endpoints, telemetry).
*   **Child module composition**: `gen submodule` orchestrates end-to-end child module generation and wiring (legacy alias: `addchild`).

## Installation

Build from source:

```bash
git clone https://github.com/matt-FFFFFF/tfmodmake.git
cd tfmodmake
go build -o tfmodmake ./cmd/tfmodmake
```

## Usage

### Base Module Generation

Generate a base Terraform module:

```bash
# Default form (no subcommand - backward compatible)
./tfmodmake -spec <path_or_url> -resource <resource_type> [flags]

# Explicit form using 'gen' subcommand (recommended)
./tfmodmake gen -spec <path_or_url> -resource <resource_type> [flags]
```

### Flags

*   `-spec`: (Required) Path or URL to the OpenAPI specification.
*   `-resource`: (Required) Resource type to generate configuration for (e.g., `Microsoft.ContainerService/managedClusters`).
*   `-root`: (Optional) Dot-separated path to the root object within the resource schema (e.g., `properties` or `properties.networkProfile`). If specified, only properties under this root are generated as variables.
*   `-local-name`: (Optional) Name of the local variable to generate in `locals.tf`. Defaults to `resource_body` or a snake_case version of the `-root` path.

**Note:** Base generation does NOT create `main.interfaces.tf` by default. Use `add avm-interfaces` (see below) to opt-in to AVM interfaces scaffolding.

### AVM Interfaces Scaffolding

Generate `main.interfaces.tf` for AVM interfaces (opt-in):

```bash
./tfmodmake add avm-interfaces [-resource <resource_type>]
```

This command creates/overwrites `main.interfaces.tf` in the current module directory. If `-resource` is not provided, it attempts to infer the resource type from the existing `main.tf` file.

**Example:**

```bash
# After generating a base module
./tfmodmake gen -spec <spec_url> -resource Microsoft.Test/testResources

# Optionally add AVM interfaces scaffolding
./tfmodmake add avm-interfaces -resource Microsoft.Test/testResources
```

### Submodule Wrapper Generation

To generate a map-based module block wrapper for an existing submodule:

```bash
# New form (recommended)
./tfmodmake add submodule <path_to_submodule>

# Legacy form (still supported)
./tfmodmake addsub <path_to_submodule>
```

This command reads the Terraform module at the specified path and generates:
1.  `variables.<module_name>.tf`: A variable accepting a map of objects matching the submodule's inputs.
2.  `main.<module_name>.tf`: A `module` block using `for_each` to iterate over the variable.


### Child Module Generation and Wiring

The `gen submodule` command orchestrates the complete process of creating a child module and wiring it into the parent module:

```bash
# New form (recommended)
./tfmodmake gen submodule -parent <parent_type> -child <child_type> [flags]

# Legacy form (still supported)
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
2.  Wires the child module into the root module using the same mechanics as `add submodule`

**Module naming convention:**

By default, the module name is derived from the last segment of the child resource type (e.g., `.../storages` → `storages`). However, following the project convention that each submodule makes one thing (singular), it's recommended to use `-module-name` to specify a singular name when the derived name is plural.

**Example:**

```bash
# Using spec-root (recommended) with singular module name
./tfmodmake gen submodule \
  -parent "Microsoft.App/managedEnvironments" \
  -child "Microsoft.App/managedEnvironments/storages" \
  -module-name "storage" \
  -spec-root "https://github.com/Azure/azure-rest-api-specs/tree/main/specification/app/resource-manager/Microsoft.App/ContainerApps"

# Using explicit spec with singular module name
./tfmodmake gen submodule \
  -parent "Microsoft.KeyVault/vaults" \
  -child "Microsoft.KeyVault/vaults/secrets" \
  -module-name "secret" \
  -spec "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/keyvault/resource-manager/Microsoft.KeyVault/stable/2024-11-01/secrets.json"

# Custom module directory and name
./tfmodmake gen submodule \
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
./tfmodmake add submodule modules/secrets

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

The base generation tool creates these files in the current directory:

1.  `variables.tf`: Contains the input variables (including `name`, `parent_id`, and `tags` when supported).
2.  `locals.tf`: Contains the local value constructing the JSON body structure.
3.  `main.tf`: Scaffold for the `azapi_resource` using the generated locals.
4.  `outputs.tf`: Outputs exposing the resource ID and name.
5.  `terraform.tf`: Terraform and provider version constraints.

**Note:** `main.interfaces.tf` is NOT generated by default. Use `add avm-interfaces` to opt-in to AVM interfaces scaffolding.

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

The `discover children` command inspects OpenAPI specs and returns child resource types that can be deployed under a parent resource.

This is a discovery process that does not generate any terraform code; it is designed to help identify child resources for use with the `gen submodule` command.

```bash
# New form (recommended)
./tfmodmake discover children -spec <path_or_url> -parent <resource_type> [-json]

# Legacy form (still supported)
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
./tfmodmake discover children \
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
