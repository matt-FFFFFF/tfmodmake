# tfmodmake Design Notes

## Overview

tfmodmake generates Terraform AzureRM modules from Azure resource type definitions (via bicep-types-az). This document captures key architectural decisions and their rationale.

---

## Core Architecture

### bicep-types-az as Source of Truth

**Decision:** Generate all Terraform code from bicep-types-az resource definitions rather than raw OpenAPI specs or handwriting modules.

**Rationale:**

- Azure bicep-types-az provides pre-generated, strongly-typed resource definitions derived from Azure REST API specs
- All references are pre-resolved, allOf merged, and metadata normalized
- No need for complex OpenAPI parsing, allOf merging, or $ref resolution
- Zero external dependencies from the bicep-types-go library

**Trade-offs:**

- bicep-types has slightly less constraint information than raw OpenAPI (no multipleOf, uniqueItems, UUID format), but the tradeoff is worth it for the massive simplification
- Generated code may need post-processing for edge cases
- Dependent on Azure's spec quality and update cadence

---

### Package Structure

**Decision:** Public packages (`bicepdata`, `schema`, `terraform`, `hclgen`, `naming`, `submodule`) with CLI in `cmd/tfmodmake`.

**Rationale:**

- `bicepdata` downloads and caches bicep-types-az type files; `schema` converts bicep types to internal representation
- Enables external use (e.g., MCP server integration, programmatic usage)
- Clear separation between library functionality and CLI concerns
- Follows Go community conventions for reusable code
- Allows other tools to import and extend functionality

---

### Multi-File Generation Strategy

**Decision:** Generate separate Terraform files: `variables.tf`, `locals.tf`, `main.tf`, `outputs.tf`, `terraform.tf`.

**Rationale:**

- Follows Azure Verified Modules (AVM) conventions
- Separates concerns (inputs → processing → resources → outputs)
- Makes generated code easier to review and understand
- Familiar structure for Terraform practitioners

**Pattern:**

```text
variables.tf  → User-facing inputs (flattened from resource type schema)
locals.tf     → Internal transformations (nested structure reconstruction)
main.tf       → Resource definitions (azapi_resource blocks)
outputs.tf    → Exported values
terraform.tf  → Provider requirements
```

---

### Schema Flattening & Reconstruction

**Decision:** Flatten nested resource type properties into top-level variables, then reconstruct in locals.

**Rationale:**

- Terraform variables don't handle deeply nested optional objects well
- Flattening provides better UX (simpler variable names, easier defaults)
- Locals reconstruct the API-required structure without user burden
- Aligns with AVM patterns and Terraform best practices

**Example:**

```hcl
# variables.tf
variable "os_profile_secrets_source_vault_id" { ... }

# locals.tf
locals {
  resource_body = {
    properties = {
      osProfile = {
        secrets = var.os_profile_secrets_source_vault_id != null ? {
          sourceVault = { id = var.os_profile_secrets_source_vault_id }
        } : null
      }
    }
  }
}
```

---

### Secret Field Handling

**Decision:** Detect secret-related fields and generate ephemeral variables with `null` defaults.

**Rationale:**

- Secrets shouldn't be stored in state or version control
- Terraform 1.10+ ephemeral values provide secure secret handling
- `null` defaults prevent accidental secret exposure
- Users must explicitly provide secrets at runtime

**Detection:** Fields marked with `Sensitive = true` on `StringType`/`ObjectType` in bicep-types, or with the `WriteOnly` property flag, are marked ephemeral.

---

### AVM Interface Capabilities

**Decision:** Use a hybrid approach combining spec-based detection with sensible ARM platform defaults.

**Rationale:**

Investigation of real OpenAPI specs vs ARM platform capabilities revealed:

1. **Private Endpoints**: ✅ Reliably detectable from specs via `privateEndpointConnections`/`privateLinkResources` paths
2. **Diagnostic Settings**: ❌ Not in resource specs (managed by `Microsoft.Insights` provider as generic ARM capability)
3. **Locks**: ❌ Not in resource specs (managed by `Microsoft.Authorization` provider, universal ARM capability)
4. **Role Assignments**: ❌ Not in resource specs (managed by `Microsoft.Authorization` provider, universal ARM capability)
5. **Customer-Managed Keys**: ⚠️ Heuristically detectable from encryption-related properties (false negatives acceptable)
6. **Managed Identities**: ✅ Reliably detectable from specs via `identity` property (mostly for parent resources)

**Default Strategy:**

**For Parent Resources:**
- Private Endpoints: Generate if detected in spec
- Diagnostic Settings: **Always generate** (works on 95%+ of ARM resources)
- Locks: **Always generate** (universal ARM platform capability)
- Role Assignments: **Always generate** (universal ARM platform capability)
- Customer-Managed Keys: Generate only if detected (opt-in feature, conservative approach)
- Managed Identities: Generate if detected in spec (part of core resource configuration)

**For Child Resources:**
- Private Endpoints: Generate if detected in spec
- Diagnostic Settings: Conditional (many children don't emit independent logs)
- Locks: **Always generate** (all ARM resources support locks)
- Role Assignments: **Always generate** (all ARM resources support RBAC)
- Customer-Managed Keys: Generate only if detected
- Managed Identities: Generate only if detected (rare for children)

**Trade-offs:**

- **Over-generation vs under-generation**: Better to generate scaffolding that users can opt out of than miss capabilities
- **ARM platform knowledge**: Locks and RBAC are universal; diagnostic settings work on nearly all top-level resources
- **Spec limitations**: Individual resource specs don't declare cross-cutting ARM concerns
- **User control**: AVM utility module is input-driven; unused variables cause no harm

**Implementation:** `SupportsManagedIdentity` is set from `ResourceSchema.SupportsIdentity` (detected by `schema.detectSupportsIdentity()`). Private endpoints and CMK detection have been de-prioritised (always false currently). The `isChild` parameter applies different ARM platform defaults.

---

### Child Resource Discovery

**Decision:** Programmatically discover child resources from bicep-types-az index using resource type hierarchy.

**Rationale:**

- Azure resource hierarchy is encoded in resource type names
- Manual maintenance of child resource lists is error-prone
- Enables automatic submodule scaffolding
- Supports AVM's parent-child module pattern

**Pattern:** Discovery uses `bicepdata.FetchIndex()` + `schema.DiscoverChildren()` to find child types. For example, `Microsoft.App/managedEnvironments/storages` is discovered as a child of `Microsoft.App/managedEnvironments`.

---

### Error Handling Philosophy

**Decision:** Return errors rather than panic for recoverable conditions.

**Rationale:**

- Panics are for programmer errors and unrecoverable failures
- Schema processing errors should be reportable to users
- Enables better testing (can assert on error conditions)
- Follows idiomatic Go error handling

**Evolution:** Phase 2 refactoring (Dec 2025) replaced 8 panics with proper error returns.

---

### Output Directory Pattern

**Decision:** Accept output directory parameter; default to current directory for base modules, `modules/` for submodules.

**Rationale:**

- Eliminates global state mutation (no `os.Chdir`)
- Enables parallel generation (if needed in future)
- Makes code more testable and predictable
- Follows AVM convention of `modules/` for child resources

**Evolution:** Originally used `generateInDirectory()` with `os.Chdir`. Refactored to explicit directory passing (Dec 2025).

---

### CLI Design Philosophy

**Decision:** Simple command-line interface using `urfave/cli/v3` framework with explicit subcommands.

**Rationale:**

- Structured CLI framework with built-in help generation and argument parsing
- Explicit commands are self-documenting
- Sufficient for current use cases (not building kubectl/docker-level complexity)
- Easy to extend with new commands as needed

**Commands:**

- `gen` - Generate base module
- `gen submodule` - Generate child module
- `discover children` - Find child resource types
- `discover versions` - List available API versions for a resource type
- `gen avm` - Orchestrate full AVM module with submodules
- `update` - Update cached bicep-types-az data

---

### Resource Type Resolution

**Decision:** Download bicep-types-az index.json at runtime; resolve resource types and API versions from the index.

**Rationale:**

- Single authoritative index of all Azure resource types and API versions
- No need to navigate GitHub tree structures or discover spec files
- Supports offline/CI use via local filesystem fallback

**Pattern:** Users provide `--resource` type and optionally `--api-version`; if no version specified, latest stable is resolved automatically. `--include-preview` allows preview versions. Local filesystem fallback supported for offline/CI use.

---

## Design Principles

1. **Generated code should be idiomatic** - Output should look hand-written
2. **Fail fast with clear errors** - Don't proceed with invalid schemas
3. **AVM compliance by default** - Follow Azure Verified Modules patterns
4. **Minimal dependencies** - Only essential libraries (bicep-types-go, HCL writer)
5. **Composable packages** - Each package has single, clear responsibility
6. **Explicit over implicit** - Prefer verbose clarity to magic behavior
7. **Schema-only validations** - Generate validations only from declarative schema constraints, not cross-field semantic rules

---

## Future Considerations

### What We're NOT Building

- **Generic Terraform generator** - Azure-specific by design
- **Plugin system** - Current scope doesn't require extensibility
- **Web UI** - CLI-first tool for developer workflows
- **State management** - Generates modules, doesn't manage infrastructure

### Potential Extensions

- **Validation rules** - Deeper schema constraint enforcement
- **Testing scaffolds** - Generate test fixtures from examples
- **Documentation generation** - Auto-generate README from schema

---

## References

- [Azure Verified Modules](https://azure.github.io/Azure-Verified-Modules/)
- [Azure REST API Specs](https://github.com/Azure/azure-rest-api-specs)
- [Azure bicep-types-az](https://github.com/Azure/bicep-types-az)
- [bicep-types-go](https://github.com/Azure/bicep-types) (Go module under `src/bicep-types-go`)
- [Terraform Module Conventions](https://www.terraform.io/docs/language/modules/develop/structure.html)
