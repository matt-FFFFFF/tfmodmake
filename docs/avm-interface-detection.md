# AVM Interface Detection: Investigation & Design

## Executive Summary

The original design assumed we could detect all AVM interface support from OpenAPI specs. After migrating to bicep-types-az resource definitions, the investigation reveals:

- ❌ **Private Endpoints**: Detection removed (de-prioritised — requires path analysis not available in bicep-types)
- ❌ **Diagnostic Settings**: Not in resource definitions (generic ARM capability)
- ❌ **Locks**: Not in resource definitions (universal ARM capability)
- ❌ **Role Assignments**: Not in resource definitions (universal ARM capability)
- ❌ **Customer-Managed Keys**: Detection removed (de-prioritised — heuristic was unreliable)
- ✅ **Managed Identities**: Reliably detectable from bicep-types-az resource definitions

**Conclusion**: Use hybrid approach combining schema-based detection (managed identity only) with ARM platform defaults. Private endpoints and CMK detection have been de-prioritised.

---

## Background

Azure Verified Modules (AVM) expect modules to provide scaffolding for common Azure interfaces:

- **Private Endpoints** (Private Link connectivity)
- **Diagnostic Settings** (monitoring/logging to Log Analytics, Storage, Event Hub)
- **Locks** (prevent accidental deletion/modification)
- **Role Assignments** (RBAC permissions)
- **Customer-Managed Keys** (CMK for encryption at rest)
- **Managed Identities** (system-assigned and user-assigned identities for authentication)

The question: Can we detect which interfaces a resource supports by analyzing its bicep-types-az resource definition?

---

## Investigation Method

1. Examined bicep-types-az resource definitions for:
   - `Microsoft.App/managedEnvironments` (Container Apps)
   - `Microsoft.ContainerService/managedClusters` (AKS)
   - `Microsoft.KeyVault/vaults` (Key Vault)
   - `Microsoft.KeyVault/vaults/secrets` (child resource example)

2. Queried azurerm provider schema to understand what it knows

3. Analyzed AVM utility module (`terraform-azure-avm-utl-interfaces`) requirements

4. Cross-referenced findings against ARM platform documentation

---

## Findings by Interface Type

### 1. Private Endpoints

**Detection Status:** ❌ **REMOVED (de-prioritised)**

**Background:**

In the previous OpenAPI-based approach, private endpoint support was detected by scanning API paths for `/privateEndpointConnections` and `/privateLinkResources` sub-resources. This was reliable because the OpenAPI specs contained full path definitions.

**Why detection was removed:**

The migration to bicep-types-az resource definitions means we no longer have access to API path information. Bicep-types defines resource schemas (ObjectType properties) but does not expose the sub-resource path hierarchy needed for private endpoint detection.

**Resources that support Private Link expose:**
- `/privateEndpointConnections` (list and manage connections)
- `/privateLinkResources` (discover available sub-resources)

This information is not represented in the bicep-types ObjectType structure.

**Current approach:** Private endpoint detection is **not auto-detected**. The `SupportsPrivateEndpoints` capability is always `false`. Users can manually add private endpoint scaffolding when needed, or the AVM interfaces module wiring will include the variable with a note that it is not auto-detected.

**Recommendation:** ❌ **Do not auto-detect** (would require a separate data source or allow-list in future)

---

### 2. Diagnostic Settings

**Resource Definition Evidence:** ❌ **NONE**

**What we DON'T see:**

No individual resource definition contains paths like:
```
/{resourceId}/providers/Microsoft.Insights/diagnosticSettings
```

**Why:** Diagnostic Settings are managed by the `Microsoft.Insights` resource provider as a **generic ARM capability**.

**Path Pattern:**
```
{resourceId}/providers/Microsoft.Insights/diagnosticSettings/{diagnosticSettingName}
```

This works on the resource ID of nearly any ARM resource without being declared in that resource's definition.

**Which resources support it?**

- Most top-level ARM resources (95%+)
- Many child resources that emit independent telemetry
- Exceptions: Some ephemeral or purely logical resources

**azurerm provider knowledge:**

The azurerm provider has resources like `azurerm_monitor_diagnostic_setting` that work on many resource types, but doesn't maintain an explicit allow-list in schema.

**AVM expectations:**

AVM utility module accepts `diagnostic_settings` variable and generates:
```hcl
resource "azapi_resource" "diagnostic_settings" {
  for_each = module.avm_interfaces.diagnostic_settings_azapi
  
  type      = "Microsoft.Insights/diagnosticSettings@2021-05-01-preview"
  parent_id = azapi_resource.this.id
  # ...
}
```

**Recommendation:** ✅ **Always generate for parent resources** (conservative over-generation acceptable)

For child resources: Use heuristic to determine if independently monitorable (or always generate with clear documentation).

---

### 3. Locks

**Resource Definition Evidence:** ❌ **NONE**

**Why:** Locks are managed by `Microsoft.Authorization` provider as a **universal ARM capability**.

**Path Pattern:**
```
{scope}/providers/Microsoft.Authorization/locks/{lockName}
```

**Scope:** Can be applied at:
- Subscription level
- Resource group level
- Individual resource level (any resource)

**azurerm provider:**

```hcl
resource "azurerm_management_lock" {
  name       = "lock-name"
  scope      = azurerm_resource.example.id  # Works on ANY resource
  lock_level = "CanNotDelete"  # or "ReadOnly"
}
```

**AVM expectations:**

```hcl
lock = {
  kind = "CanNotDelete"  # or "ReadOnly" or "None"
  name = "lock-name"     # optional, auto-generated if not provided
}
```

**Recommendation:** ✅ **Always generate** (universal ARM capability, no false positives)

---

### 4. Role Assignments

**Resource Definition Evidence:** ❌ **NONE**

**Why:** Role assignments are managed by `Microsoft.Authorization` provider as a **universal ARM capability**.

**Path Pattern:**
```
{scope}/providers/Microsoft.Authorization/roleAssignments/{roleAssignmentId}
```

**Scope:** Can be applied at any ARM scope (subscription, RG, resource).

**azurerm provider:**

```hcl
resource "azurerm_role_assignment" {
  scope                = azurerm_resource.example.id  # Works on ANY resource
  role_definition_name = "Contributor"
  principal_id         = "..."
}
```

**AVM expectations:**

```hcl
role_assignments = {
  "deployment_user" = {
    principal_id               = "..."
    role_definition_id_or_name = "Contributor"
  }
}
```

**Recommendation:** ✅ **Always generate** (universal ARM capability, no false positives)

---

### 5. Customer-Managed Keys (CMK)

**Detection Status:** ❌ **REMOVED (de-prioritised)**

**Background:**

In the previous OpenAPI-based approach, CMK support was heuristically detected by scanning request body schemas for encryption-related properties (`encryption`, `customerManagedKey`, `diskEncryption`, etc.).

**Why detection was removed:**

The heuristic detection was unreliable due to:

1. **Naming variations**: `encryption`, `customerManagedKey`, `diskEncryption`, `dataEncryption`, etc.
2. **Not always in PUT body**: Some resources configure encryption via separate operations
3. **Platform vs customer-managed**: Definitions may mention encryption without supporting CMK

While it would be technically possible to apply similar heuristics to bicep-types ObjectType properties, the false positive/negative rate was deemed too high to justify the effort.

**Current approach:** CMK detection is **not auto-detected**. The `SupportsCustomerManagedKey` capability is always `false`. Users can manually add CMK scaffolding when needed.

**Recommendation:** ❌ **Do not auto-detect** (heuristic was unreliable, manual override preferred)

---

### 6. Managed Identities

**bicep-types-az Evidence:** ✅ **STRONG**

**What we see:**

In the bicep-types-az resource definitions, resources that support managed identity have an `identity` property at the top level of their ObjectType. This property contains child properties including `type` (the identity type enum) and `userAssignedIdentities`.

**Pattern:** Resources supporting managed identity have a writable `identity` property with writable `type` or `userAssignedIdentities` children in the bicep-types ObjectType definition.

**Reliability:** Very high. Observed consistently across:
- Container Apps Managed Environments ✅
- AKS Managed Clusters ✅
- Key Vault Vaults ✅
- Key Vault Secrets (child) ❌ (not supported)

**azurerm provider schema:**

```hcl
resource "azurerm_container_app_environment" {
  # ...
  identity {
    type         = "SystemAssigned, UserAssigned"  # or "SystemAssigned" or "UserAssigned"
    identity_ids = [...]  # For UserAssigned
  }
}
```

**AVM expectations:**

```hcl
managed_identities = {
  system_assigned            = true
  user_assigned_resource_ids = [
    "/subscriptions/.../resourceGroups/.../providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity"
  ]
}
```

**Special considerations:**

1. **Part of core resource, not extension**: Unlike other AVM interfaces, managed identity is typically configured as part of the resource's request body, not as a separate extension resource.

2. **Often required for other features**: Resources may need managed identity to:
   - Access customer-managed keys in Key Vault
   - Authenticate to other Azure services
   - Use specific Azure features (e.g., AKS pod identity)

3. **Mostly for parents**: Child resources rarely have their own managed identities - they typically inherit authentication context from their parent.

**Detection method:**

Detection is implemented in `schema/convert.go` via `detectSupportsIdentity()`, which runs during resource schema conversion from bicep-types-az:

```go
// detectSupportsIdentity checks if the resource supports managed identity configuration.
// This looks for writable "identity.type" or "identity.userAssignedIdentities" properties.
func detectSupportsIdentity(rs *ResourceSchema) bool {
    identityProp, ok := rs.Properties["identity"]
    if !ok || identityProp.ReadOnly {
        return false
    }

    if identityProp.Children == nil {
        return false
    }

    // Check for writable "type" sub-property
    typeProp, hasType := identityProp.Children["type"]
    if hasType && !typeProp.ReadOnly {
        return true
    }

    // Check for writable "userAssignedIdentities" sub-property
    uaiProp, hasUAI := identityProp.Children["userAssignedIdentities"]
    if hasUAI && !uaiProp.ReadOnly {
        return true
    }

    return false
}
```

The result is stored in `ResourceSchema.SupportsIdentity` and consumed by the generator via `terraform.SupportsIdentity()`.

**Recommendation:** ✅ **Use schema-based detection** (high confidence, mostly for parent resources)

---

## AVM Utility Module Expectations

The `terraform-azure-avm-utl-interfaces` module:

**Required inputs:**
- `parent_id` - Resource ID of parent resource
- `this_resource_id` - Resource ID of this resource

**Optional inputs (with defaults):**
- `diagnostic_settings = {}` - Map of diagnostic setting configurations
- `private_endpoints = {}` - Map of private endpoint configurations
- `customer_managed_key = null` - Customer managed key configuration
- `lock = null` - Lock configuration
- `role_assignments = {}` - Map of role assignment configurations
- `managed_identities = {}` - Managed identity configuration (system-assigned and/or user-assigned)

**Key insight:** The module is **input-driven**. It generates resources only for variables that are provided. Unused variables cause no harm.

---

## Comparison: bicep-types-az vs azurerm Provider

### What azurerm Knows

The azurerm provider doesn't explicitly advertise interface support in resource schemas. It's **implicit knowledge**:

1. **Private Endpoints**: Inferred from Azure Private Link service registration
2. **Diagnostic Settings**: Works on most resources via `azurerm_monitor_diagnostic_setting`
3. **Locks**: Works on all resources via `azurerm_management_lock`
4. **Role Assignments**: Works on all resources via `azurerm_role_assignment`
5. **CMK**: Resource-specific schema properties when supported

The provider relies on ARM platform behavior and manual mapping, not definition-driven discovery.

---

## Recommendations

### Parent Resources (Top-Level Resources)

**Default Position:** **ENABLE ALL EXCEPT PE AND CMK** (unless manually overridden)

| Interface | Detection Strategy | Default | Rationale |
|-----------|-------------------|---------|-----------|
| **Private Endpoints** | ❌ Not auto-detected | Do not generate | Detection removed (requires path analysis not in bicep-types) |
| **Diagnostic Settings** | ❌ Cannot detect | ✅ **ALWAYS generate** | Works on 95%+ of ARM resources, over-generation acceptable |
| **Locks** | ❌ Cannot detect | ✅ **ALWAYS generate** | Universal ARM capability, no false positives |
| **Role Assignments** | ❌ Cannot detect | ✅ **ALWAYS generate** | Universal ARM capability, no false positives |
| **Customer-Managed Keys** | ❌ Not auto-detected | Do not generate | Detection removed (heuristic unreliable) |
| **Managed Identities** | ✅ Detect from schema | Generate if detected | Part of core resource, reliably detectable from bicep-types |

**Benefits:**
- Modules work out-of-box for most common scenarios
- Users can disable via `null` or empty map values
- Better discoverability (scaffolding shows what's possible)

---

### Child Resources (Submodules)

**Default Position:** **MORE CONSERVATIVE**

| Interface | Detection Strategy | Default | Rationale |
|-----------|-------------------|---------|-----------|
| **Private Endpoints** | ❌ Not auto-detected | Do not generate | Detection removed; rare for children anyway |
| **Diagnostic Settings** | ❌ Cannot detect | ⚠️ **CONDITIONAL** | Many children don't emit independent logs |
| **Locks** | ❌ Cannot detect | ✅ **ALWAYS generate** | Can lock individual child resources |
| **Role Assignments** | ❌ Cannot detect | ✅ **ALWAYS generate** | RBAC applies to children |
| **Customer-Managed Keys** | ❌ Not auto-detected | Do not generate | Detection removed; rare for child resources |
| **Managed Identities** | ✅ Detect from schema | Generate only if detected | Very rare for children, parent provides auth context |

**Diagnostic Settings considerations for children:**

Most child resources don't emit independent diagnostic logs (parent aggregates them). However:

- Some children ARE independently monitorable (e.g., AKS node pools)
- Better to over-generate with clear docs than miss capabilities
- Users can easily remove unused scaffolding

**Suggested approach:** Always generate with documentation noting that many child resources don't support diagnostic settings independently.

---

## Implementation Strategy

### Code Structure

```go
// InterfaceCapabilities represents which AVM interface scaffolding should be generated.
// Defined in terraform/generator.go
type InterfaceCapabilities struct {
    SupportsPrivateEndpoints   bool
    SupportsDiagnostics        bool
    SupportsCustomerManagedKey bool
    SupportsManagedIdentity    bool
}
```

Interface capabilities are built from the `schema.ResourceSchema` during generation.
Only `SupportsManagedIdentity` is currently auto-detected; the other three fields default to `false`:

```go
// In terraform/generator.go — generateWithOpts()
func generateWithOpts(o *generatorOptions) error {
    supportsIdentity := SupportsIdentity(o.schema) // reads rs.SupportsIdentity

    // Build interface capabilities from schema
    caps := InterfaceCapabilities{
        SupportsManagedIdentity: supportsIdentity,
        // SupportsPrivateEndpoints, SupportsDiagnostics, SupportsCustomerManagedKey
        // are not auto-detected and default to false.
    }
    // ...
}

// SupportsIdentity reports whether the schema supports configuring managed identity.
func SupportsIdentity(rs *schema.ResourceSchema) bool {
    return rs != nil && rs.SupportsIdentity
}
```

The identity detection itself lives in `schema/convert.go` and runs at schema conversion time:

```go
// In schema/convert.go — called during ConvertResource()
rs.SupportsIdentity = detectSupportsIdentity(rs)
```

See the [Managed Identities](#6-managed-identities) section above for the full `detectSupportsIdentity()` implementation.

### CLI Flags (Future)

Allow users to override detection:

```bash
tfmodmake gen avm Microsoft.App/managedEnvironments \
  --force-diagnostics    # Override: always generate
  --skip-diagnostics     # Override: never generate
  --force-cmk            # Override: generate even if not detected
  --force-pe             # Override: generate private endpoint scaffolding
```

---

## Edge Cases & Limitations

### 1. Diagnostic Log Categories

**Problem:** Resource definitions don't tell us which log categories a resource supports.

**Solution:** Let users provide category names. AVM utility module handles validation at runtime via Azure API.

```hcl
diagnostic_settings = {
  "default" = {
    log_categories = ["ContainerAppConsoleLogs", "ContainerAppSystemLogs"]
    workspace_resource_id = azurerm_log_analytics_workspace.example.id
  }
}
```

---

### 2. Private Endpoint Sub-Resources

**Problem:** Some resources support Private Endpoints on multiple sub-resources (e.g., Storage Account supports `blob`, `file`, `queue`, `table`).

Since private endpoint detection has been de-prioritised, this is deferred to a future enhancement. When private endpoint scaffolding is manually added, users specify `subresource_names` in the variable:

```hcl
private_endpoints = {
  "pe1" = {
    subnet_resource_id = "..."
    subresource_names  = ["blob"]  # User chooses
  }
}
```

---

### 3. Customer-Managed Key Variations

**Problem:** Encryption patterns vary widely across resource types.

Since CMK detection has been de-prioritised, this is deferred to a future enhancement. When CMK scaffolding is manually added, it uses the generic AVM utility module pattern.

---

### 4. Child Resource Monitoring

**Problem:** No resource definition indicator for which children emit independent logs.

**Heuristics that might help:**
- Child has state machines or processing (likely monitorable)
- Child is purely configuration/metadata (likely not monitorable)
- Child exposes metric endpoints (likely monitorable)

**Current approach:** Generate diagnostic settings for all children with clear documentation:

```hcl
# Note: Not all child resources support diagnostic settings independently.
# If this resource's diagnostic data is aggregated by its parent, you can
# remove this block or set diagnostic_settings = {} to disable it.
```

---

## Testing Strategy

### Unit Tests

Test detection functions against known bicep-types-az resource definitions:

```go
func TestConvertResource_SupportsIdentity(t *testing.T) {
    // Test resources with writable identity.type → SupportsIdentity = true
    // Test resources with writable identity.userAssignedIdentities → SupportsIdentity = true
    // Test resources with read-only identity → SupportsIdentity = false
    // Test resources without identity property → SupportsIdentity = false
}
```

### Integration Tests

Generate full modules and verify:

1. Generated variables match expected interfaces
2. `terraform validate` passes
3. AVM utility module integration works

```go
func TestInterfacesGeneration_ManagedEnvironments(t *testing.T) {
    // Generate module
    // Check for presence of:
    // - var.private_endpoints
    // - var.diagnostic_settings
    // - var.lock
    // - var.role_assignments
    // Verify main.interfaces.tf uses AVM utility module
}
```

---

## Migration Plan

### Phase 1: Migrate to bicep-types-az ✅ (Complete)

- [x] Replace OpenAPI-based detection with bicep-types-az schema analysis
- [x] Implement `detectSupportsIdentity()` in `schema/convert.go`
- [x] De-prioritise private endpoint detection (not available in bicep-types)
- [x] De-prioritise CMK detection (heuristic unreliable)
- [x] Update `InterfaceCapabilities` struct (removed `SupportsLocks`, `SupportsRoleAssignments` — ARM defaults handled elsewhere)

### Phase 2: Update Code Generation

- [ ] Generate locks variables for all resources
- [ ] Generate role assignments variables for all resources
- [ ] Generate diagnostic settings for all parents (conditional for children)
- [ ] Update `main.interfaces.tf` template to include all interfaces

### Phase 3: Documentation

- [ ] Update README with interface detection strategy
- [ ] Add comments in generated code explaining each interface
- [ ] Document override patterns for edge cases

### Phase 4: CLI Enhancements

- [ ] Add `--force-diagnostics` / `--skip-diagnostics` flags
- [ ] Add `--force-cmk` flag
- [ ] Add `--force-pe` flag
- [ ] Add `--skip-interfaces` for minimal generation

---

## Decision Record

**Date:** 2025-12-29 (original), updated 2026-03 for bicep-types-az migration

**Decision:** Adopt hybrid detection strategy combining schema-based detection with ARM platform defaults. After migrating from OpenAPI to bicep-types-az, only managed identity is auto-detected from resource definitions.

**Rationale:**

1. **Data source change**: Migration from OpenAPI specs to bicep-types-az removed access to API path information needed for private endpoint detection
2. **Managed identity**: Reliably detectable from bicep-types ObjectType properties (writable `identity` property)
3. **CMK heuristic**: De-prioritised due to unreliable detection across resource types
4. **Completeness**: ARM platform capabilities (locks, RBAC, diagnostics) are universal and don't require detection
5. **Usability**: Over-generation of ARM defaults is acceptable when users can easily opt out

**For Parent Resources:**
- ❌ Private Endpoints: Not auto-detected (de-prioritised)
- ✅ Diagnostic Settings: Always generate (ARM platform default)
- ✅ Locks: Always generate (universal ARM)
- ✅ Role Assignments: Always generate (universal ARM)
- ❌ Customer-Managed Keys: Not auto-detected (de-prioritised)
- ✅ Managed Identities: Detect from schema (high confidence)

**For Child Resources:**
- ❌ Private Endpoints: Not auto-detected (de-prioritised)
- ✅ Diagnostic Settings: Always generate with documentation
- ✅ Locks: Always generate (universal ARM)
- ✅ Role Assignments: Always generate (universal ARM)
- ❌ Customer-Managed Keys: Not auto-detected (de-prioritised)
- ✅ Managed Identities: Detect from schema (rare for children)

**Alternatives Considered:**

1. **OpenAPI spec-only detection**: Abandoned — migrated to bicep-types-az which doesn't expose API paths
2. **All-or-nothing generation**: Rejected due to loss of granularity
3. **Runtime Azure API queries**: Deferred to future (adds complexity and auth requirements)
4. **Hardcoded allow-lists**: Rejected due to maintenance burden
5. **Heuristic CMK/PE detection on bicep-types**: De-prioritised — effort/reliability trade-off not justified

**Status:** Approved, implementation in progress

---

## References

- [Azure Verified Modules](https://azure.github.io/Azure-Verified-Modules/)
- [AVM Interfaces Utility Module](https://github.com/Azure/terraform-azure-avm-utl-interfaces)
- [bicep-types-az](https://github.com/Azure/bicep-types-az) — resource type definitions used for schema analysis
- [ARM Template Reference - Locks](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/lock-resources)
- [ARM Template Reference - Role Assignments](https://learn.microsoft.com/en-us/azure/role-based-access-control/overview)
- [ARM Template Reference - Diagnostic Settings](https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/diagnostic-settings)
- [Azure Private Link](https://learn.microsoft.com/en-us/azure/private-link/)
