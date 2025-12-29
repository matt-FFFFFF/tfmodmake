# allOf Handling in tfmodmake

## Overview

tfmodmake implements robust `allOf` handling to correctly process Azure OpenAPI specifications that use schema composition. This document explains how `allOf` schemas are flattened and merged during code generation.

## What is allOf?

In OpenAPI/JSON Schema, `allOf` represents schema composition where an object must satisfy ALL of the listed subschemas. It's commonly used in Azure specs for:

- **Inheritance**: Base resource types extended by specific resource types
- **Composition**: Combining multiple property groups into a single schema
- **Common types**: Referencing shared schemas from external files

### Example

```json
{
  "allOf": [
    { "$ref": "#/definitions/Resource" },
    {
      "type": "object",
      "properties": {
        "properties": {
          "type": "object",
          "properties": {
            "kubernetesVersion": { "type": "string" }
          }
        }
      }
    }
  ]
}
```

## Effective Schema Behavior

### Property Merging

When multiple `allOf` components define properties, they are merged into a single effective schema:

```javascript
// Component 1
{ properties: { name: { type: "string" } } }

// Component 2  
{ properties: { age: { type: "integer" } } }

// Result
{ properties: { 
    name: { type: "string" },
    age: { type: "integer" }
  }
}
```

### Required Field Union

Required fields from all components are combined (union operation):

```javascript
// Component 1
{ required: ["name"] }

// Component 2
{ required: ["age", "email"] }

// Result
{ required: ["age", "email", "name"] }  // Sorted alphabetically
```

### ReadOnly Fields

Fields marked as `readOnly` are preserved in the merged schema but are excluded from Terraform input generation by the existing `isWritableProperty` checks:

```javascript
// Component with readOnly field
{
  properties: {
    id: { type: "string", readOnly: true },
    name: { type: "string" }
  },
  required: ["id", "name"]
}

// Result: Both properties present in merged schema
// Only "name" appears in generated Terraform variables
```

### Conflict Detection

If the same property name appears in multiple components with incompatible schemas, generation fails with a detailed error:

```
Error: conflicting definitions for property "count" in allOf:
component 1 defines it differently than previous definition.
First defined in schema with type=integer, description="Count as integer";
conflicting definition has type=string, description="Count as string"
```

**Equivalence checking** is tolerant of documentation differences:
- Different `description` values are OK
- Different `title` values are OK  
- Different extension values (x-ms-*) for docs are OK
- But structural differences (type, format, constraints) cause errors

### Recursive Processing

`allOf` handling is applied recursively:
- nested object properties
- array item schemas
- additionalProperties schemas

This matters because Azure specs often compose *nested* object shapes (not just the resource root).

### Cycle Handling

The implementation uses cache-based memoization to handle:
- Recursive structures (e.g., error details containing error details)
- Shared schemas referenced in multiple places
- Preventing infinite loops

Each schema is processed once and cached. Subsequent references return the cached result.

## Integration Points

### 1. Non-Destructive Shape Generation

**Shape consumers** (types/locals/variables) use helper functions that return effective properties and required fields without mutating the original schema:

```go
// In generate_variables.go, generate_locals.go, validations.go
effectiveProps, err := openapi.GetEffectiveProperties(schema)
effectiveRequired, err := openapi.GetEffectiveRequired(schema)
```

These functions:
- Merge properties and required fields from all `allOf` components
- Use internal caching and cycle detection
- Return errors for conflicts or cycles (treated as fatal)
- Preserve the original schema for validation generation

### 2. Constraint Generation (Validation Blocks)

**Constraint consumers** continue to use the original schema with `resolveSchemaForValidation()`:

```go
// In validations.go
childSchema := resolveSchemaForValidation(prop.Value)
```

This function applies "most restrictive wins" semantics for constraints (min/max, enum, etc.) by examining the original `allOf` array, ensuring validation blocks have correct constraint merging per PR #20.

### 3. No Global Flattening

The original schema is preserved throughout the generation pipeline:
- No per-navigation-step flattening in `NavigateSchema`
- Properties accessed on-demand via helper functions

## Testing

The implementation includes comprehensive tests:

### Unit Tests for GetEffectiveProperties/Required (production code path)
In `internal/openapi/allof_effective_test.go`:
- ✅ Simple allOf composition
- ✅ No allOf (direct return)
- ✅ Conflict detection with clear errors
- ✅ Cycle detection (A→B→A)
- ✅ Nested allOf
- ✅ Required field union
- ✅ Required field deduplication
- ✅ Nested allOf for required
- ✅ Memoization across multiple references

### Integration Tests
- ✅ Real Azure AKS spec generation (uses allOf extensively)
- ✅ Real Azure Container Apps spec generation
- ✅ ReadOnly required fields excluded from Terraform variables

## Real-World Examples

### Azure AKS managedClusters

Uses `allOf` to combine:
- Base `Resource` type (id, name, type, location, tags)
- `TrackedResource` extensions
- `ManagedCluster` specific properties

After applying effective `allOf` shape merging, all properties are available for Terraform generation.

#### Why this exists: a concrete, user-visible win (agent pool profiles)

The AKS Managed Cluster spec composes agent pool shape via `allOf`. Without `allOf` shape merging, tfmodmake only sees the *top-level* `properties` on a schema, and will miss properties declared in `allOf` components.

That leads to a very practical failure mode: the module exposes only a tiny subset of the real agent pool configuration surface.

When generating `Microsoft.ContainerService/managedClusters` from:

```
https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/containerservice/resource-manager/Microsoft.ContainerService/aks/stable/2025-10-01/managedClusters.json
```

we observed this before/after in the generated Terraform.

**Before (no effective `allOf` merge):** `agent_pool_profiles` contains only `name`.

```hcl
variable "agent_pool_profiles" {
  type = list(object({
    name = string
  }))
}
```

**After (effective `allOf` merge):** `agent_pool_profiles` exposes the real shape (a small sample shown here).

```hcl
variable "agent_pool_profiles" {
  type = list(object({
    name                = string
    enable_auto_scaling = optional(bool)
    min_count           = optional(number)
    max_count           = optional(number)
    vm_size             = optional(string)
    vnet_subnet_id      = optional(string)
    node_labels         = optional(map(string))
    kubelet_config      = optional(object({
      cpu_manager_policy = optional(string)
      pod_max_pids       = optional(number)
    }))
  }))
}
```

This isn’t “just typing”: the locals wiring also starts sending these fields into the request body (showing the same sample fields):

```hcl
agentPoolProfiles = var.agent_pool_profiles == null ? null : [for item in var.agent_pool_profiles : {
  name              = item.name
  enableAutoScaling = item.enable_auto_scaling
  minCount          = item.min_count
  maxCount          = item.max_count
  vmSize            = item.vm_size
  vnetSubnetID      = item.vnet_subnet_id
  nodeLabels        = item.node_labels
  kubeletConfig     = item.kubelet_config == null ? null : {
    cpuManagerPolicy = item.kubelet_config.cpu_manager_policy
    podMaxPids       = item.kubelet_config.pod_max_pids
  }
}]
```

Net effect: `allOf` shape merging prevents a class of “missing configuration surface” bugs that are otherwise very hard to diagnose (because the spec is valid, but the generator is blind to composed properties).

### Azure Container Apps managedEnvironments

Similarly uses `allOf` for composition. The flattening correctly merges common types with resource-specific properties.

Note: for some resources (including managedEnvironments), `allOf` is often used primarily for inheriting base resource metadata (`TrackedResource` / `ProxyResource`). Since many inherited fields are `readOnly` (or already handled via dedicated top-level variables like `location`/`tags`), you may not see large new input surfaces from `allOf` in those cases.

## Performance

The cache-based approach ensures:
- Each schema is processed at most once
- No exponential blowup from recursive structures
- Fast lookups for shared schemas

## Future Enhancements

Potential improvements (not currently needed):
- Support for `oneOf` and `anyOf` (not commonly used in Azure specs)
- Merge validation constraints from components (currently handled in validations.go)
- Performance metrics for large specs
