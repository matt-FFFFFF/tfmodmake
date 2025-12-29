# AVM Interface Detection: Investigation & Design

## Executive Summary

The original design assumed we could detect all AVM interface support from OpenAPI specs. Investigation reveals:

- ✅ **Private Endpoints**: Reliably detectable from specs
- ❌ **Diagnostic Settings**: Not in specs (generic ARM capability)
- ❌ **Locks**: Not in specs (universal ARM capability)
- ❌ **Role Assignments**: Not in specs (universal ARM capability)  
- ⚠️ **Customer-Managed Keys**: Heuristically detectable (false negatives acceptable)- ✅ **Managed Identities**: Reliably detectable from specs (mostly for parents)
**Conclusion**: Use hybrid approach combining spec-based detection with ARM platform defaults.

---

## Background

Azure Verified Modules (AVM) expect modules to provide scaffolding for common Azure interfaces:

- **Private Endpoints** (Private Link connectivity)
- **Diagnostic Settings** (monitoring/logging to Log Analytics, Storage, Event Hub)
- **Locks** (prevent accidental deletion/modification)
- **Role Assignments** (RBAC permissions)
- **Customer-Managed Keys** (CMK for encryption at rest)
- **Managed Identities** (system-assigned and user-assigned identities for authentication)

The question: Can we detect which interfaces a resource supports by analyzing its OpenAPI specification?

---

## Investigation Method

1. Examined real OpenAPI specs for:
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

**OpenAPI Spec Evidence:** ✅ **STRONG**

**What we see:**

```json
{
  "paths": {
    "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/privateEndpointConnections": {
      "get": { ... }
    },
    "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/privateEndpointConnections/{privateEndpointConnectionName}": {
      "get": { ... },
      "put": { ... },
      "delete": { ... }
    },
    "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App/managedEnvironments/{environmentName}/privateLinkResources": {
      "get": { ... }
    }
  }
}
```

**Pattern:** Resources supporting Private Link expose:
- `/privateEndpointConnections` (list and manage connections)
- `/privateLinkResources` (discover available sub-resources)

**Reliability:** Very high. Observed consistently across:
- Container Apps Managed Environments ✅
- AKS Managed Clusters ✅
- Key Vault Vaults ✅

**Detection method:**

```go
func detectPrivateEndpointSupport(spec *openapi3.T, resourceType string) bool {
    for path := range spec.Paths.Map() {
        pathLower := strings.ToLower(path)
        if strings.Contains(pathLower, "privateendpointconnections") ||
           strings.Contains(pathLower, "privatelinkresources") {
            return true
        }
    }
    return false
}
```

**Recommendation:** ✅ **Use spec-based detection**

---

### 2. Diagnostic Settings

**OpenAPI Spec Evidence:** ❌ **NONE**

**What we DON'T see:**

No individual resource spec contains paths like:
```
/{resourceId}/providers/Microsoft.Insights/diagnosticSettings
```

**Why:** Diagnostic Settings are managed by the `Microsoft.Insights` resource provider as a **generic ARM capability**.

**Path Pattern:**
```
{resourceId}/providers/Microsoft.Insights/diagnosticSettings/{diagnosticSettingName}
```

This works on the resource ID of nearly any ARM resource without being declared in that resource's spec.

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

**OpenAPI Spec Evidence:** ❌ **NONE**

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

**OpenAPI Spec Evidence:** ❌ **NONE**

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

**OpenAPI Spec Evidence:** ⚠️ **MODERATE / HEURISTIC**

**What we see:**

Some resource specs include encryption-related properties:

```json
{
  "definitions": {
    "DiskEncryptionConfiguration": {
      "type": "object",
      "properties": {
        "diskEncryptionKeyUrl": { "type": "string" },
        "identity": { "$ref": "#/definitions/ManagedServiceIdentity" }
      }
    },
    "ManagedEnvironmentProperties": {
      "properties": {
        "diskEncryption": {
          "$ref": "#/definitions/DiskEncryptionConfiguration"
        }
      }
    }
  }
}
```

**Challenges:**

1. **Naming variations**: `encryption`, `customerManagedKey`, `diskEncryption`, `dataEncryption`, etc.
2. **Not always in PUT body**: Some resources configure encryption via separate operations
3. **Platform vs customer-managed**: Specs may mention encryption without supporting CMK

**Detection approach:**

```go
func detectCustomerManagedKeySupport(spec *openapi3.T, resourceType string) bool {
    // Search request body schemas for encryption-related properties
    for path, pathItem := range spec.Paths.Map() {
        if pathItem.Put == nil { continue }
        
        schema := extractRequestBodySchema(pathItem.Put)
        if hasEncryptionProperty(schema) {
            return true
        }
    }
    return false
}

func hasEncryptionProperty(schema *openapi3.Schema) bool {
    props, _ := GetEffectiveProperties(schema)
    for name, propRef := range props {
        nameLower := strings.ToLower(name)
        if nameLower == "encryption" || 
           nameLower == "customermanagedkey" ||
           strings.Contains(nameLower, "encryptionkey") {
            return true
        }
        // Recurse into nested properties object
        if name == "properties" {
            if hasEncryptionProperty(propRef.Value) {
                return true
            }
        }
    }
    return false
}
```

**Observed results:**

- Container Apps Managed Environments: ✅ Detected (`diskEncryption`)
- AKS Managed Clusters: ⚠️ May be detected (has disk encryption options)
- Key Vault: ❌ Not detected (KV manages keys, doesn't use CMK itself)

**Recommendation:** ⚠️ **Use heuristic detection, accept false negatives**

CMK is an opt-in security feature. Conservative detection (only when clearly indicated) is appropriate. Users can manually add CMK variables if needed.

---

### 6. Managed Identities

**OpenAPI Spec Evidence:** ✅ **STRONG**

**What we see:**

```json
{
  "definitions": {
    "ManagedEnvironment": {
      "properties": {
        "identity": {
          "description": "Managed identities for the Managed Environment to interact with other Azure services without maintaining any secrets or credentials in code.",
          "$ref": "../../../../../../common-types/resource-management/v5/managedidentity.json#/definitions/ManagedServiceIdentity"
        }
      }
    }
  }
}
```

**Common type definition:**

```json
{
  "ManagedServiceIdentity": {
    "type": "object",
    "description": "Managed service identity (system assigned and/or user assigned identities)",
    "properties": {
      "principalId": {
        "type": "string",
        "format": "uuid",
        "readOnly": true
      },
      "tenantId": {
        "type": "string",
        "format": "uuid",
        "readOnly": true  
      },
      "type": {
        "$ref": "#/definitions/ManagedServiceIdentityType"
      },
      "userAssignedIdentities": {
        "$ref": "#/definitions/UserAssignedIdentities"
      }
    },
    "required": ["type"]
  }
}
```

**Pattern:** Resources supporting managed identity have an `identity` property at the top level of their schema.

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

```go
func detectManagedIdentitySupport(spec *openapi3.T, resourceType string) bool {
    // Look for "identity" property in PUT request body schema
    for path, pathItem := range spec.Paths.Map() {
        if pathItem.Put != nil && matchesResourceType(path, resourceType) {
            schema := extractRequestBodySchema(pathItem.Put)
            if hasIdentityProperty(schema) {
                return true
            }
        }
    }
    return false
}

func hasIdentityProperty(schema *openapi3.Schema) bool {
    props, _ := GetEffectiveProperties(schema)
    for name := range props {
        if strings.ToLower(name) == "identity" {
            return true
        }
    }
    return false
}
```

**Recommendation:** ✅ **Use spec-based detection** (high confidence, mostly for parent resources)

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
- `role_assignments = {}` - Map of role assignment configurations- `managed_identities = {}` - Managed identity configuration (system-assigned and/or user-assigned)
**Key insight:** The module is **input-driven**. It generates resources only for variables that are provided. Unused variables cause no harm.

---

## Comparison: Specs vs azurerm Provider

### What azurerm Knows

The azurerm provider doesn't explicitly advertise interface support in resource schemas. It's **implicit knowledge**:

1. **Private Endpoints**: Inferred from Azure Private Link service registration
2. **Diagnostic Settings**: Works on most resources via `azurerm_monitor_diagnostic_setting`
3. **Locks**: Works on all resources via `azurerm_management_lock`
4. **Role Assignments**: Works on all resources via `azurerm_role_assignment`
5. **CMK**: Resource-specific schema properties when supported

The provider relies on ARM platform behavior and manual mapping, not spec-driven discovery.

---

## Recommendations

### Parent Resources (Top-Level Resources)

**Default Position:** **ENABLE ALL EXCEPT CMK** (unless detected)

| Interface | Detection Strategy | Default | Rationale |
|-----------|-------------------|---------|-----------|
| **Private Endpoints** | ✅ Detect from spec paths | Generate if detected | High reliability, few false positives |
| **Diagnostic Settings** | ❌ Cannot detect | ✅ **ALWAYS generate** | Works on 95%+ of ARM resources, over-generation acceptable |
| **Locks** | ❌ Cannot detect | ✅ **ALWAYS generate** | Universal ARM capability, no false positives |
| **Role Assignments** | ❌ Cannot detect | ✅ **ALWAYS generate** | Universal ARM capability, no false positives |
| **Customer-Managed Keys** | ⚠️ Heuristic detection | Generate only if detected | Opt-in feature, false negatives acceptable |
| **Managed Identities** | ✅ Detect from spec schema | Generate if detected | Part of core resource, reliably detectable |

**Benefits:**
- Modules work out-of-box for most common scenarios
- Users can disable via `null` or empty map values
- Better discoverability (scaffolding shows what's possible)

---

### Child Resources (Submodules)

**Default Position:** **MORE CONSERVATIVE**

| Interface | Detection Strategy | Default | Rationale |
|-----------|-------------------|---------|-----------|
| **Private Endpoints** | ✅ Detect from spec paths | Generate if detected | Rare for children but possible |
| **Diagnostic Settings** | ❌ Cannot detect | ⚠️ **CONDITIONAL** | Many children don't emit independent logs |
| **Locks** | ❌ Cannot detect | ✅ **ALWAYS generate** | Can lock individual child resources |
| **Role Assignments** | ❌ Cannot detect | ✅ **ALWAYS generate** | RBAC applies to children |
| **Customer-Managed Keys** | ⚠️ Heuristic detection | Generate only if detected | Rare for child resources |
| **Managed Identities** | ✅ Detect from spec schema | Generate only if detected | Very rare for children, parent provides auth context |

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
// InterfaceCapabilities represents which AVM interfaces should be generated
type InterfaceCapabilities struct {
    SupportsPrivateEndpoints   bool
    SupportsDiagnostics        bool
    SupportsCustomerManagedKey bool
    SupportsManagedIdentity    bool
    SupportsLocks              bool
    SupportsRoleAssignments    bool
}

// DetectInterfaceCapabilities analyzes spec and applies ARM platform defaults
func DetectInterfaceCapabilities(spec *openapi3.T, resourceType string, isChild bool) InterfaceCapabilities {
    caps := InterfaceCapabilities{
        // Spec-based detection
        SupportsPrivateEndpoints:   detectPrivateEndpointSupport(spec, resourceType),
        SupportsCustomerManagedKey: detectCustomerManagedKeySupport(spec, resourceType),
        SupportsManagedIdentity:    detectManagedIdentitySupport(spec, resourceType),
        
        // ARM platform defaults
        SupportsLocks:             true, // Universal capability
        SupportsRoleAssignments:   true, // Universal capability
    }
    
    // Diagnostic settings: different defaults for parents vs children
    if isChild {
        // Conservative for children - could use heuristic or always true
        caps.SupportsDiagnostics = true  // With clear documentation
    } else {
        // Always true for parent resources
        caps.SupportsDiagnostics = true
    }
    
    return caps
}
```

### CLI Flags (Future)

Allow users to override detection:

```bash
tfmodmake gen avm \
  -resource Microsoft.App/managedEnvironments \
  --force-diagnostics    # Override: always generate
  --skip-diagnostics     # Override: never generate
  --force-cmk            # Override: generate even if not detected
```

---

## Edge Cases & Limitations

### 1. Diagnostic Log Categories

**Problem:** Specs don't tell us which log categories a resource supports.

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

**Problem:** Some resources support Private Endpoints on multiple sub-resources.

Example: Storage Account supports:
- `blob` - Blob storage endpoint
- `file` - File storage endpoint
- `queue` - Queue storage endpoint
- `table` - Table storage endpoint

**What specs show:**

```json
{
  "paths": {
    "/{resourceId}/privateLinkResources": {
      "get": {
        "responses": {
          "200": {
            "schema": {
              "value": [
                { "properties": { "groupId": "blob" } },
                { "properties": { "groupId": "file" } },
                ...
              ]
            }
          }
        }
      }
    }
  }
}
```

**Solution:** Generate generic private endpoint scaffolding. Users specify `subresource_names` in variable:

```hcl
private_endpoints = {
  "pe1" = {
    subnet_resource_id = "..."
    subresource_names  = ["blob"]  # User chooses
  }
}
```

Runtime discovery of available sub-resources would require Azure API queries (future enhancement).

---

### 3. Customer-Managed Key Variations

**Problem:** Encryption patterns vary widely:

- Disk encryption (VM disks, temp storage)
- Data encryption at rest (databases, storage)
- Transport encryption (TLS/SSL)
- Double encryption (platform + customer keys)

Different key storage:
- Azure Key Vault
- Azure Managed HSM
- Azure Dedicated HSM

**Current approach:** Detect presence of encryption properties, generate generic CMK scaffolding.

**Improvement:** Document per-service encryption patterns in generated code comments.

---

### 4. Child Resource Monitoring

**Problem:** No spec indicator for which children emit independent logs.

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

Test detection functions against known spec patterns:

```go
func TestDetectPrivateEndpoints(t *testing.T) {
    tests := []struct{
        name     string
        specPath string
        want     bool
    }{
        {"managedEnvironments", "specs/managedEnvironments.json", true},
        {"managedClusters", "specs/managedClusters.json", true},
        {"resourceGroups", "specs/resourceGroups.json", false},
    }
    // ...
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

### Phase 1: Update Detection Logic ✅ (Current)

- [x] Add `isChild` parameter to `DetectInterfaceCapabilities()`
- [x] Update detection to use ARM platform defaults
- [x] Add `SupportsLocks` and `SupportsRoleAssignments` fields

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
- [ ] Add `--skip-interfaces` for minimal generation

---

## Decision Record

**Date:** 2025-12-29

**Decision:** Adopt hybrid detection strategy combining spec-based detection with ARM platform defaults.

**Rationale:**

1. **Accuracy**: Specs reliably indicate Private Endpoint support
2. **Completeness**: ARM platform capabilities (locks, RBAC) are universal
3. **Usability**: Over-generation is acceptable when users can easily opt out
4. **Maintainability**: Defaults reduce need for per-resource customization

**For Parent Resources:**
- ✅ Private Endpoints: Detect from specs
- ✅ Diagnostic Settings: Always generate (ARM platform default)
- ✅ Locks: Always generate (universal ARM)
- ✅ Role Assignments: Always generate (universal ARM)
- ⚠️ Customer-Managed Keys: Detect from specs (heuristic)
- ✅ Managed Identities: Detect from specs (high confidence)

**For Child Resources:**
- ✅ Private Endpoints: Detect from specs
- ✅ Diagnostic Settings: Always generate with documentation
- ✅ Locks: Always generate (universal ARM)
- ✅ Role Assignments: Always generate (universal ARM)
- ⚠️ Customer-Managed Keys: Detect from specs (heuristic)
- ✅ Managed Identities: Detect from specs (rare for children)

**Alternatives Considered:**

1. **Spec-only detection**: Rejected due to missing cross-cutting ARM capabilities
2. **All-or-nothing generation**: Rejected due to loss of granularity
3. **Runtime Azure API queries**: Deferred to future (adds complexity and auth requirements)
4. **Hardcoded allow-lists**: Rejected due to maintenance burden

**Status:** Approved, implementation in progress

---

## References

- [Azure Verified Modules](https://azure.github.io/Azure-Verified-Modules/)
- [AVM Interfaces Utility Module](https://github.com/Azure/terraform-azure-avm-utl-interfaces)
- [Azure REST API Specs](https://github.com/Azure/azure-rest-api-specs)
- [ARM Template Reference - Locks](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/lock-resources)
- [ARM Template Reference - Role Assignments](https://learn.microsoft.com/en-us/azure/role-based-access-control/overview)
- [ARM Template Reference - Diagnostic Settings](https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/diagnostic-settings)
- [Azure Private Link](https://learn.microsoft.com/en-us/azure/private-link/)
