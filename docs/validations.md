# Validation Block Generation

This document describes the validation block generation feature that automatically creates Terraform validation blocks based on OpenAPI/Swagger schema constraints.

## Overview

The tool now generates validation blocks for Terraform variables based on constraints defined in the OpenAPI specification. This helps catch invalid inputs early and provides better user experience with clear error messages.

## Supported Constraint Types

### 1. String Validations

#### minLength
Validates minimum string length.

**OpenAPI:**
```json
{
  "type": "string",
  "minLength": 3
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.name == null || length(var.name) >= 3
  error_message = "name must have a minimum length of 3."
}
```

#### maxLength
Validates maximum string length.

**OpenAPI:**
```json
{
  "type": "string",
  "maxLength": 100
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.name == null || length(var.name) <= 100
  error_message = "name must have a maximum length of 100."
}
```

#### format (UUID only)
Validates UUID format using regex.

**OpenAPI:**
```json
{
  "type": "string",
  "format": "uuid"
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.id == null || can(regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", var.id))
  error_message = "id must be a valid UUID."
}
```

#### pattern
Validates string against a regular expression pattern.

**OpenAPI:**
```json
{
  "type": "string",
  "pattern": "^[a-zA-Z0-9-_]{1,63}$"
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.resource_name == null || can(regex("^[a-zA-Z0-9-_]{1,63}$", var.resource_name))
  error_message = "resource_name must match the pattern: ^[a-zA-Z0-9-_]{1,63}$."
}
```

### 2. Array/List Validations

#### minItems
Validates minimum number of array items.

**OpenAPI:**
```json
{
  "type": "array",
  "minItems": 1,
  "items": {"type": "string"}
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tags == null || length(var.tags) >= 1
  error_message = "tags must have at least 1 item(s)."
}
```

#### maxItems
Validates maximum number of array items.

**OpenAPI:**
```json
{
  "type": "array",
  "maxItems": 10,
  "items": {"type": "string"}
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tags == null || length(var.tags) <= 10
  error_message = "tags must have at most 10 item(s)."
}
```

#### uniqueItems
Validates that array items are unique.

**OpenAPI:**
```json
{
  "type": "array",
  "uniqueItems": true,
  "items": {"type": "string"}
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.unique_list == null || length(distinct(var.unique_list)) == length(var.unique_list)
  error_message = "unique_list must contain unique items."
}
```

### 3. Numeric Validations

#### minimum
Validates minimum numeric value.

**OpenAPI:**
```json
{
  "type": "integer",
  "minimum": 1
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.count == null || var.count >= 1
  error_message = "count must be greater than or equal to 1."
}
```

#### maximum
Validates maximum numeric value.

**OpenAPI:**
```json
{
  "type": "number",
  "maximum": 100
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.percentage == null || var.percentage <= 100
  error_message = "percentage must be less than or equal to 100."
}
```

#### exclusiveMinimum
Validates exclusive minimum (value must be greater than, not equal to).

**OpenAPI:**
```json
{
  "type": "number",
  "minimum": 0,
  "exclusiveMinimum": true
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.rating == null || var.rating > 0
  error_message = "rating must be greater than 0."
}
```

#### exclusiveMaximum
Validates exclusive maximum (value must be less than, not equal to).

**OpenAPI:**
```json
{
  "type": "integer",
  "maximum": 10,
  "exclusiveMaximum": true
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.scale == null || var.scale < 10
  error_message = "scale must be less than 10."
}
```

#### multipleOf
Validates that a number is a multiple of the specified value.

**OpenAPI:**
```json
{
  "type": "integer",
  "multipleOf": 5
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.size == null || abs(mod(var.size, 5)) < 0.000001
  error_message = "size must be a multiple of 5."
}
```

### 4. Enum Validations

Enum validations are generated for properties with restricted value sets.

#### Direct enum
**OpenAPI:**
```json
{
  "type": "string",
  "enum": ["Free", "Basic", "Premium"]
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tier == null || contains(["Basic", "Free", "Premium"], var.tier)
  error_message = "tier must be one of: [\"Basic\", \"Free\", \"Premium\"]."
}
```

#### Enum via allOf
**OpenAPI:**
```json
{
  "allOf": [
    {"type": "string"},
    {"enum": ["Enabled", "Disabled"]}
  ]
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.status == null || contains(["Disabled", "Enabled"], var.status)
  error_message = "status must be one of: [\"Disabled\", \"Enabled\"]."
}
```

#### Azure x-ms-enum extension
**OpenAPI:**
```json
{
  "type": "string",
  "x-ms-enum": {
    "name": "SkuName",
    "values": [
      {"value": "Free"},
      {"value": "Basic"}
    ]
  }
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.sku == null || contains(["Basic", "Free"], var.sku)
  error_message = "sku must be one of: [\"Basic\", \"Free\"]."
}
```

## Design Principles

### Null-Safety
All validations for optional fields use null-safe conditions:
```hcl
var.field == null || <validation logic>
```

This allows `null` values for optional fields while still validating provided values.

### Required Fields
Required fields don't include the null check:
```hcl
<validation logic>  # No "var.field == null ||" prefix
```

### Enum Ordering
Enum values are sorted alphabetically for stable, predictable output:
```hcl
contains(["Basic", "Free", "Premium", "Standard"], var.tier)
```

### Conservative Format Validation
Only UUID format is currently validated to avoid false positives. Other formats are not validated by default.

### Human-Readable Error Messages
Error messages are clear and actionable:
- "name must have a minimum length of 3."
- "count must be greater than or equal to 1."
- "tags must contain unique items."

## Limitations

1. **Nested property validations**: Only top-level variables receive validation blocks. Nested object properties within complex types do not get individual validations.

  Nested object validations are generated conservatively for object-typed variables: scalar fields and arrays of scalars may receive validations when they are represented as direct attributes on `var.<object>.<field>`. Deeply nested structures are not exhaustively validated.

2. **Format validation**: Only UUID format is validated. Other formats (email, date-time, etc.) are not validated by default.

3. **Read-only properties**: Validations are not generated for read-only properties as they cannot be set by users.

## Examples

### Real-World Azure Spec Example

Using the AKS managedClusters specification:
```bash
./tfmodmake \
  -spec https://raw.githubusercontent.com/Azure/azure-rest-api-specs/main/specification/containerservice/resource-manager/Microsoft.ContainerService/aks/stable/2025-10-01/managedClusters.json \
  -resource Microsoft.ContainerService/managedClusters
```

This generates validations for enum fields like `publicNetworkAccess` and `supportPlan`:
```hcl
validation {
  condition     = var.public_network_access == null || contains(["Disabled", "Enabled"], var.public_network_access)
  error_message = "public_network_access must be one of: [\"Disabled\"], \"Enabled\"."
}
```

## Testing

The validation generation feature is thoroughly tested:
- 16 unit tests covering all constraint types
- Integration tests with comprehensive scenarios
- Real-world spec testing with Azure resources
- All existing tests continue to pass (no regressions)

## Future Enhancements

Potential future improvements:
1. Additional format validators (email, date-time, etc.)
2. Nested property validations (with opt-in to control verbosity)
3. Custom validation extensions via configuration
