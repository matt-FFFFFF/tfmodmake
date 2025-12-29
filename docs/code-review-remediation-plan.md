# tfmodmake Code Review & Remediation Plan

**Review Date:** 28 December 2025 (Updated: 29 December 2025)  
**Reviewer:** Critical Code Review Agent  
**Repository:** tfmodmake - OpenAPI to Terraform Module Generator

---

## Executive Summary

**Status:** ✅ Major refactoring phases complete!

### Completed Work (29 December 2025)
- ✅ **Phase 0: Package Restructuring** - All packages moved out of `internal/`, enabling external reusability
- ✅ **Phase 1: Quick Wins** - Legacy commands removed, helper wrappers eliminated, LoadResourceFromSpecs extracted (~183 lines saved)
- ✅ **Phase 2: Error Handling** - All 8 panics replaced with proper error returns

### Current State
- All tests passing (`go test ./...` + `make test-examples`)
- Code is more idiomatic and maintainable
- Ready for external use (e.g., MCP server integration)

### Remaining High-Value Items
The following items represent potential future improvements, **prioritized by value**:

---

## 1. Remaining Code Duplication (LOW PRIORITY)

### 1.1 Spec Resolution Pattern

**Location:** `cmd/tfmodmake/main.go` - multiple command handlers

Commands like `children`, `add submodule`, and `gen avm` have nearly identical spec resolution logic (~25 lines each, appears 3 times). Could be extracted to a `resolveSpecs()` helper function.

**Impact:** ~70 lines saved, better consistency.

**Status:** Low priority - duplication is manageable and localized. Consider if touching those commands anyway.

---

## 2. Directory Changing Anti-Pattern (LOW PRIORITY)

**Location:** `cmd/tfmodmake/main.go` - `generateInDirectory()`

**Issue:** Changing the current working directory is a global side effect that can cause issues in concurrent scenarios and makes code harder to reason about.

**Evidence:**
```go
func generateInDirectory(dir string, fn func() error) error {
    originalDir, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("failed to get current directory: %w", err)
    }
    
    if err := os.Chdir(dir); err != nil {
        return fmt.Errorf("failed to change to directory %s: %w", dir, err)
    }
    
    defer func() {
        if chErr := os.Chdir(originalDir); chErr != nil {
            log.Printf("Warning: failed to restore directory to %s: %v", originalDir, chErr)
        }
    }()
    
    return fn()
}
```

**Recommendation:**
Instead of changing directories, pass an output directory parameter to the generation functions:

```go
// Modify terraform.Generate signature:
func Generate(
    schema *openapi3.Schema, 
    resourceType string, 
    localName string, 
    apiVersion string, 
    supportsTags bool, 
    supportsLocation bool, 
    nameSchema *openapi3.Schema, 
    spec *openapi3.T,
    outputDir string,  // NEW PARAMETER
) error {
    // Write files to outputDir instead of current directory
}

// Then in generateChildModule:
err := terraform.Generate(
    schema, childType, localName, apiVersion, 
    supportsTags, supportsLocation, nameSchema, doc,
    modulePath,  // Output directory
)
```

**Impact:**
- Eliminates global state mutation
- Makes code more testable (can run in parallel)
- Removes need for directory restoration logic
- Requires updating all `Generate()` calls and file writing code

---

### 3.3 String-Based Resource Type Inference (LOW PRIORITY)

**Location:** `cmd/tfmodmake/main.go` - `inferResourceTypeFromMainTf()`

**Issue:** Uses basic string searching instead of proper HCL parsing:

```go
func inferResourceTypeFromMainTf() (string, error) {
    data, err := os.ReadFile("main.tf")
    if err != nil {
        return "", fmt.Errorf("could not read main.tf: %w", err)
    }
    
    content := string(data)
    lines := strings.Split(content, "\n")
    
    for _, line := range lines {
        trimmed := strings.TrimSpace(line)
        if strings.HasPrefix(trimmed, "type") && strings.Contains(trimmed, "=") {
            parts := strings.Split(trimmed, "\"")
            if len(parts) >= 2 {
                resourceType := parts[1]
                if strings.Contains(resourceType, "Microsoft.") {
                    return resourceType, nil
                }
            }
        }
    }
    
    return "", fmt.Errorf("could not find resource type in main.tf")
}
```

**Recommendation:**
Use the HCL parser library already in dependencies:

```go
func inferResourceTypeFromMainTf() (string, error) {
    data, err := os.ReadFile("main.tf")
    if err != nil {
        return "", fmt.Errorf("could not read main.tf: %w", err)
    }
    
    file, diags := hclwrite.ParseConfig(data, "main.tf", hcl.Pos{Line: 1, Column: 1})
    if diags.HasErrors() {
        return "", fmt.Errorf("failed to parse main.tf: %w", diags)
    }
    
    for _, block := range file.Body().Blocks() {
        if block.Type() != "resource" {
            continue
        }
        if len(block.Labels()) < 2 || block.Labels()[0] != "azapi_resource" {
            continue
        }
        
        typeAttr := block.Body().GetAttribute("type")
        if typeAttr == nil {
            continue
        }
        
        // Extract string value from attribute
        val, diags := typeAttr.Expr.Value(nil)
        if diags.HasErrors() || val.Type() != cty.String {
            continue
        }
        
        typeStr := val.AsString()
        // Strip @apiVersion suffix if present
        if idx := strings.Index(typeStr, "@"); idx > 0 {
            typeStr = typeStr[:idx]
        }
        
        return typeStr, nil
    }
    
    return "", fmt.Errorf("could not find azapi_resource block with type in main.tf")
}
```

**Impact:**
- More robust parsing (handles comments, multi-line attributes, etc.)
- Correctly handles edge cases (quoted strings, escaped characters)
- Better error messages
- ~30 lines → ~40 lines but more correct

---

## 4. Command-Line Interface Design

### 4.1 Flag Parsing Complexity (MEDIUM PRIORITY)

**Location:** `cmd/tfmodmake/main.go`

**Issue:** Each command creates its own `flag.FlagSet` with duplicated flag definitions. This leads to:
- Duplication of flag definitions (e.g., `spec`, `spec-root`, `include-preview` appear in multiple commands)
- Inconsistent help text
- Manual args offset calculation

**Evidence:**
```go
// handleChildrenCommand
childrenCmd := flag.NewFlagSet(cmdName, flag.ExitOnError)
var specs stringSliceFlag
childrenCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
specRoot := childrenCmd.String("spec-root", "", "GitHub tree URL...")
// ... more flags

// handleAddChildCommand  
addChildCmd := flag.NewFlagSet(cmdName, flag.ExitOnError)
var specs stringSliceFlag
addChildCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
specRoot := addChildCmd.String("spec-root", "", "GitHub tree URL...")
// ... same flags repeated
```

**Recommendation:**
Consider using a proper CLI library like `cobra` or at minimum, extract common flag groups:

```go
type CommonSpecFlags struct {
    Specs          stringSliceFlag
    SpecRoot       string
    IncludePreview bool
    IncludeGlob    string
}

func addCommonSpecFlags(fs *flag.FlagSet) *CommonSpecFlags {
    flags := &CommonSpecFlags{}
    fs.Var(&flags.Specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
    fs.StringVar(&flags.SpecRoot, "spec-root", "", "GitHub tree URL under Azure/azure-rest-api-specs")
    fs.BoolVar(&flags.IncludePreview, "include-preview", false, "Include latest preview API version")
    fs.StringVar(&flags.IncludeGlob, "include", "*.json", "Glob filter for spec files")
    return flags
}

// Then in each command:
func handleChildrenCommand() {
    childrenCmd := flag.NewFlagSet("discover children", flag.ExitOnError)
    specFlags := addCommonSpecFlags(childrenCmd)
    parent := childrenCmd.String("parent", "", "Parent resource type")
    // ... command-specific flags
}
```

**Impact:**
- Reduces duplication by ~50 lines
- Ensures consistent flag behavior across commands
- Easier to add new common flags
- Better foundation for future CLI improvements

---

### 4.2 Missing Command Validation (LOW PRIORITY)

**Location:** `cmd/tfmodmake/main.go` - Multiple handlers

**Issue:** Commands don't validate mutually exclusive flags or required flag combinations consistently.

**Example:**
```go
// In handleGenAVMCommand, both validation messages are identical:
if *resourceType == "" {
    log.Fatalf("Usage: ... -resource is required")
}
if len(specs) == 0 && *specRoot == "" {
    log.Fatalf("Usage: ... At least one -spec or -spec-root is required")
}
```

But in `handleAddChildCommand`, the validation is slightly different, and some commands allow flags that don't make sense together.

**Recommendation:**
Create a validation helper:

```go
type FlagValidator struct {
    errors []string
}

func (v *FlagValidator) RequireOneOf(name1, name2 string, val1, val2 interface{}) {
    empty1 := isZero(val1)
    empty2 := isZero(val2)
    if empty1 && empty2 {
        v.errors = append(v.errors, fmt.Sprintf("at least one of -%s or -%s is required", name1, name2))
    }
}

func (v *FlagValidator) Require(name string, val interface{}) {
    if isZero(val) {
        v.errors = append(v.errors, fmt.Sprintf("-%s is required", name))
    }
}

func (v *FlagValidator) MutuallyExclusive(name1, name2 string, val1, val2 interface{}) {
    if !isZero(val1) && !isZero(val2) {
        v.errors = append(v.errors, fmt.Sprintf("-%s and -%s are mutually exclusive", name1, name2))
    }
}

func (v *FlagValidator) FailIfErrors(cmdName string) {
    if len(v.errors) == 0 {
        return
    }
    fmt.Fprintf(os.Stderr, "Error: invalid flags for %s:\n", cmdName)
    for _, err := range v.errors {
        fmt.Fprintf(os.Stderr, "  - %s\n", err)
    }
    os.Exit(1)
}
```

**Impact:**
- Consistent validation across commands
- Better error messages (can report multiple issues at once)
- Self-documenting flag requirements

---

## 5. Architecture & Design Improvements

### 5.1 Missing Abstraction for Generation Pipeline (MEDIUM PRIORITY)

**Location:** Multiple generation functions in `internal/terraform`

**Issue:** The generation pipeline (variables → locals → main → outputs → interfaces) is implicitly ordered but not explicitly modeled. The `Generate()` function is a simple sequence of calls with no pipeline abstraction.

**Current Design:**
```go
func Generate(schema *openapi3.Schema, ...) error {
    if err := generateTerraform(); err != nil {
        return err
    }
    if err := generateVariables(...); err != nil {
        return err
    }
    if hasSchema {
        if err := generateLocals(...); err != nil {
            return err
        }
    }
    if err := generateMain(...); err != nil {
        return err
    }
    if err := generateOutputs(...); err != nil {
        return err
    }
    return nil
}
```


**Current State:** Changes the working directory globally using `os.Chdir()`, which:
- Affects all goroutines (not thread-safe)
- Makes code harder to test
- Hides directory dependencies
- Can cause issues if not properly restored

**Recommendation:** Pass the output directory explicitly to all generation functions instead. This is a larger refactor but makes the code more maintainable.

**Status:** Low priority - current implementation works, but consider for future refactoring.

---

## 3. Testing & Quality Improvements

### 3.1 CLI Integration Tests

**Current State:**
- ✅ Good unit test coverage for packages
- ✅ `make test-examples` validates real-world generation scenarios
- Missing: End-to-end CLI command tests

**Recommendation:** Consider adding CLI integration tests if commands become more complex. Current `make test-examples` provides good coverage for generation workflow.

**Status:** Low priority - existing testing is adequate.
