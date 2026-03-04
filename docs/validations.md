# Validation Block Generation

This document describes the validation block generation feature that automatically creates Terraform validation blocks based on bicep-types-az resource type constraints.

## Overview

The tool generates validation blocks for Terraform variables based on constraints defined in bicep-types-az type definitions. This helps catch invalid inputs early and provides better user experience with clear error messages.

## Supported Constraint Types

### 1. String Validations

#### MinLength
Validates minimum string length.

**bicep-types:**
```go
// StringType from bicep-types-go
&types.StringType{
    MinLength: ptr(int64(3)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.name == null || length(var.name) >= 3
  error_message = "name must have a minimum length of 3."
}
```

#### MaxLength
Validates maximum string length.

**bicep-types:**
```go
// StringType from bicep-types-go
&types.StringType{
    MaxLength: ptr(int64(100)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.name == null || length(var.name) <= 100
  error_message = "name must have a maximum length of 100."
}
```

#### Pattern
Validates string against a regular expression pattern.

**bicep-types:**
```go
// StringType from bicep-types-go
&types.StringType{
    Pattern: "^[a-zA-Z0-9-_]{1,63}$",
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

#### MinLength (minItems)
Validates minimum number of array items. In bicep-types, array item count constraints use `MinLength`/`MaxLength` on `ArrayType`.

**bicep-types:**
```go
// ArrayType from bicep-types-go
&types.ArrayType{
    ItemType:  &types.TypeReference{Type: stringTypeIndex},
    MinLength: ptr(int64(1)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tags == null || length(var.tags) >= 1
  error_message = "tags must have at least 1 item(s)."
}
```

#### MaxLength (maxItems)
Validates maximum number of array items.

**bicep-types:**
```go
// ArrayType from bicep-types-go
&types.ArrayType{
    ItemType:  &types.TypeReference{Type: stringTypeIndex},
    MaxLength: ptr(int64(10)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tags == null || length(var.tags) <= 10
  error_message = "tags must have at most 10 item(s)."
}
```

### 3. Numeric Validations

#### MinValue
Validates minimum numeric value.

**bicep-types:**
```go
// IntegerType from bicep-types-go
&types.IntegerType{
    MinValue: ptr(int64(1)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.count == null || var.count >= 1
  error_message = "count must be greater than or equal to 1."
}
```

#### MaxValue
Validates maximum numeric value.

**bicep-types:**
```go
// IntegerType from bicep-types-go
&types.IntegerType{
    MaxValue: ptr(int64(100)),
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.percentage == null || var.percentage <= 100
  error_message = "percentage must be less than or equal to 100."
}
```

### 4. Enum Validations

Enum validations are generated for properties with restricted value sets. In bicep-types, enums are represented as a `UnionType` containing references to `StringLiteralType` entries.

**bicep-types:**
```go
// UnionType referencing StringLiteralType entries
// Given types array containing:
//   [5] = &types.StringLiteralType{Value: "Free"}
//   [6] = &types.StringLiteralType{Value: "Basic"}
//   [7] = &types.StringLiteralType{Value: "Premium"}
&types.UnionType{
    Elements: []*types.TypeReference{
        {Type: 5},
        {Type: 6},
        {Type: 7},
    },
}
```

**Generated Terraform:**
```hcl
validation {
  condition     = var.tier == null || contains(["Basic", "Free", "Premium"], var.tier)
  error_message = "tier must be one of: [\"Basic\", \"Free\", \"Premium\"]."
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

### Human-Readable Error Messages
Error messages are clear and actionable:
- "name must have a minimum length of 3."
- "count must be greater than or equal to 1."
- "tier must be one of: [\"Basic\", \"Free\", \"Premium\"]."

## Limitations

1. **Nested property validations**: Only top-level variables receive validation blocks. Nested object properties within complex types do not get individual validations.

  Nested object validations are generated conservatively for object-typed variables: scalar fields and arrays of scalars may receive validations when they are represented as direct attributes on `var.<object>.<field>`. Deeply nested structures are not exhaustively validated.

2. **uniqueItems validation**: Not available. bicep-types-az does not expose a `uniqueItems` constraint, so duplicate-item checks cannot be generated.

3. **multipleOf validation**: Not available. bicep-types-az does not expose a `multipleOf` constraint.

4. **exclusiveMinimum / exclusiveMaximum**: These are pre-resolved by the bicep-types generator — the bounds are already adjusted so only `>=` and `<=` comparisons are generated. There is no separate exclusive-bound check.

5. **UUID format validation**: Not available. bicep-types-az does not surface string format information, so UUID format checks are not generated.

6. **Read-only properties**: Validations are not generated for read-only properties as they cannot be set by users.

## Examples

### Real-World Azure Spec Example

Using the AKS managedClusters resource type:
```bash
./tfmodmake \
  --resource Microsoft.ContainerService/managedClusters \
  --api-version 2025-10-01
```

This generates validations for enum fields like `publicNetworkAccess` and `supportPlan`:
```hcl
validation {
  condition     = var.public_network_access == null || contains(["Disabled", "Enabled"], var.public_network_access)
  error_message = "public_network_access must be one of: [\"Disabled\", \"Enabled\"]."
}
```

## Testing

The validation generation feature is tested with:
- Unit tests covering all supported constraint types (string, integer, array, enum)
- Integration tests with comprehensive scenarios
- Real-world resource type testing with Azure bicep-types-az data

## Future Enhancements

Potential future improvements:
1. Nested property validations (with opt-in to control verbosity)
2. Custom validation extensions via configuration
