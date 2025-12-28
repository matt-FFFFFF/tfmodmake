# tfmodmake Code Review & Remediation Plan

**Review Date:** 28 December 2025  
**Reviewer:** Critical Code Review Agent  
**Repository:** tfmodmake - OpenAPI to Terraform Module Generator

---

## Executive Summary

This document presents a comprehensive analysis of the tfmodmake codebase with specific recommendations for improving maintainability, reducing technical debt, and adhering to idiomatic Go patterns. The tool is well-architected overall but has accumulated some technical debt in the form of:

- **Legacy command handling** that can be removed
- **Duplicate code** across multiple files
- **Non-idiomatic Go patterns** (excessive panics, helper wrapper functions)
- **Command-line argument parsing** that could be simplified
- **Missing abstractions** for common patterns

**Key Finding:** The codebase is production-quality with good test coverage, but would benefit from consolidation and modernization efforts, particularly around CLI command handling and error management.

---

## 1. Legacy Code & Backward Compatibility Issues

### 1.1 Legacy Command Aliases (HIGH PRIORITY)

**Location:** `cmd/tfmodmake/main.go`

**Issue:** The code maintains backward compatibility with old command names that are no longer documented:
- `addsub` (replaced by `add submodule`)
- `addchild` (replaced by `gen submodule`)
- `children` (replaced by `discover children`)

**Evidence:**
```go
// Lines 151-157: Legacy addsub handling
if len(os.Args) > 1 && os.Args[1] == "addsub" {
    // Legacy: tfmodmake addsub <path>
    addSub := flag.NewFlagSet("addsub", flag.ExitOnError)
    ...
}

// Lines 315-320: Legacy children handling
if len(os.Args) > 1 && os.Args[1] == "children" {
    // Legacy: tfmodmake children ...
    cmdName = "children"
    argsOffset = 2
}

// Lines 413-418: Legacy addchild handling
if len(os.Args) > 1 && os.Args[1] == "addchild" {
    // Legacy: tfmodmake addchild ...
    cmdName = "addchild"
    argsOffset = 2
}
```

**Recommendation:**
Since there is **no requirement for backward compatibility within the tool**, remove all legacy command handling:

- Remove the `addsub` command completely
- Remove the standalone `children` command
- Remove the `addchild` command
- Update any remaining references or tests

**Impact:** Simplifies `main.go` by ~60 lines, reduces cognitive load, eliminates dual code paths.

**Implementation Steps:**
1. Remove legacy command checks in `handleAddSubCommand()`
2. Remove legacy command checks in `handleChildrenCommand()`
3. Remove legacy command checks in `handleAddChildCommand()`
4. Search for and remove any tests exercising legacy commands
5. Verify all examples in README use new command syntax

---

## 2. Code Duplication

### 2.1 Spec Loading and Resource Finding Pattern (HIGH PRIORITY)

**Location:** `cmd/tfmodmake/main.go` - `generateChildModule()` and `generateBaseModule()`

**Issue:** Both functions contain nearly identical logic for:
- Iterating through specs
- Loading OpenAPI documents
- Finding resources
- Handling errors
- Applying writability overrides

**Evidence:**
```go
// generateChildModule (lines 560-620) - ~60 lines
for _, specPath := range specs {
    loadedDoc, err := openapi.LoadSpec(specPath)
    if err != nil {
        loadErrors = append(loadErrors, ...)
        continue
    }
    foundSchema, err := openapi.FindResource(loadedDoc, childType)
    if err != nil {
        searchErrors = append(searchErrors, ...)
        continue
    }
    // ... same pattern ...
}

// generateBaseModule (lines 880-940) - ~60 lines  
for _, specPath := range specSources {
    loadedDoc, err := openapi.LoadSpec(specPath)
    if err != nil {
        loadErrors = append(loadErrors, ...)
        continue
    }
    foundSchema, err := openapi.FindResource(loadedDoc, resourceType)
    if err != nil {
        searchErrors = append(searchErrors, ...)
        continue
    }
    // ... same pattern ...
}
```

**Recommendation:**
Extract a reusable function in `cmd/tfmodmake/main.go` or better yet, `internal/cli/loader.go`:

**File:** `internal/cli/loader.go` (new file)
```go
package cli

import (
    "fmt"
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/matt-FFFFFF/tfmodmake/openapi"
    "github.com/matt-FFFFFF/tfmodmake/terraform"
)

// ResourceLoadResult contains all information needed to generate a Terraform module
// for a resource loaded from OpenAPI specs.
type ResourceLoadResult struct {
    Schema           *openapi3.Schema
    NameSchema       *openapi3.Schema
    Doc              *openapi3.T
    APIVersion       string
    SupportsTags     bool
    SupportsLocation bool
}

// LoadResourceFromSpecs attempts to find and load a resource type from a list of specs.
// It returns the first successful match or an error with details about failures.
func LoadResourceFromSpecs(specs []string, resourceType string) (*ResourceLoadResult, error) {
    var loadErrors []string
    var searchErrors []string
    
    for _, specPath := range specs {
        loadedDoc, err := openapi.LoadSpec(specPath)
        if err != nil {
            loadErrors = append(loadErrors, fmt.Sprintf("- %s: %v", specPath, err))
            continue
        }
        
        foundSchema, err := openapi.FindResource(loadedDoc, resourceType)
        if err != nil {
            searchErrors = append(searchErrors, fmt.Sprintf("- %s: %v", specPath, err))
            continue
        }
        
        // Found the resource! Build result
        result := &ResourceLoadResult{
            Schema: foundSchema,
            Doc:    loadedDoc,
        }
        
        if loadedDoc.Info != nil {
            result.APIVersion = loadedDoc.Info.Version
        }
        
        result.NameSchema, _ = openapi.FindResourceNameSchema(loadedDoc, resourceType)
        
        openapi.AnnotateSchemaRefOrigins(result.Schema)
        if resolver, err := openapi.NewPropertyWritabilityResolver(specPath); err == nil && resolver != nil {
            openapi.ApplyPropertyWritabilityOverrides(result.Schema, resolver)
        }
        
        result.SupportsTags = terraform.SupportsTags(result.Schema)
        result.SupportsLocation = terraform.SupportsLocation(result.Schema)
        
        return result, nil
    }
    
    return nil, buildResourceNotFoundError(resourceType, loadErrors, searchErrors)
}

func buildResourceNotFoundError(resourceType string, loadErrors, searchErrors []string) error {
    errMsg := fmt.Sprintf("resource type %s not found in any of the provided specs", resourceType)
    if len(loadErrors) > 0 {
        errMsg += fmt.Sprintf("\n\nSpec load errors:\n%s", strings.Join(loadErrors, "\n"))
    }
    if len(searchErrors) > 0 {
        errMsg += fmt.Sprintf("\n\nSpecs checked:\n%s", strings.Join(searchErrors, "\n"))
    }
    return fmt.Errorf("%s", errMsg)
}
```

**Then in `cmd/tfmodmake/main.go`:**
```go
// generateChildModule - replace lines 560-620 with:
result, err := cli.LoadResourceFromSpecs(specs, childType)
if err != nil {
    return fmt.Errorf("failed to load child resource: %w", err)
}

schema := result.Schema
doc := result.Doc
apiVersion := result.APIVersion
nameSchema := result.NameSchema
supportsTags := result.SupportsTags
supportsLocation := result.SupportsLocation

// generateBaseModule - replace lines 880-940 with:
result, err := cli.LoadResourceFromSpecs(specSources, resourceType)
if err != nil {
    return fmt.Errorf("failed to load resource: %w", err)
}

schema := result.Schema
doc := result.Doc
apiVersion := result.APIVersion
nameSchema := result.NameSchema
supportsTags := result.SupportsTags
supportsLocation := result.SupportsLocation
```

**Implementation Steps:**
1. After Phase 0 completes, create `internal/cli/loader.go`
2. Move duplicated logic to `LoadResourceFromSpecs()`
3. Update `generateChildModule()` to use new function
4. Update `generateBaseModule()` to use new function
5. Remove duplicated error building code
6. Add tests in `internal/cli/loader_test.go`

**Impact:** Eliminates ~120 lines of duplication, centralizes error handling logic.

---

### 2.2 Spec Resolution Pattern Duplication (MEDIUM PRIORITY)

**Location:** `cmd/tfmodmake/main.go` - Multiple command handlers

**Issue:** The spec resolution pattern appears in multiple commands with slight variations:

```go
// handleChildrenCommand (lines 350-380)
githubToken := githubTokenFromEnv()
includeGlobs := []string{*includeGlob}
if *includeGlob == "*.json" && *parent != "" {
    includeGlobs = defaultDiscoveryGlobsForParent(*parent)
}
resolver := defaultSpecResolver{}
resolveReq := ResolveRequest{...}
resolved, err := resolver.Resolve(context.Background(), resolveReq)

// handleAddChildCommand (lines 450-475) - Same pattern
githubToken := githubTokenFromEnv()
includeGlobs := []string{*includeGlob}
if *includeGlob == "*.json" && *parent != "" {
    includeGlobs = defaultDiscoveryGlobsForParent(*parent)
}
resolver := defaultSpecResolver{}
resolveReq := ResolveRequest{...}
resolved, err := resolver.Resolve(context.Background(), resolveReq)

// handleGenAVMCommand (lines 750-770) - Same pattern again
```

**Recommendation:**
Create a helper function in the cmd package:

```go
type SpecResolveParams struct {
    Seeds          []string
    GitHubRoot     string
    DiscoverSeeds  bool
    IncludeGlob    string
    IncludePreview bool
    ParentType     string  // for glob inference
}

func resolveSpecs(params SpecResolveParams) ([]string, error) {
    githubToken := githubTokenFromEnv()
    
    includeGlobs := []string{params.IncludeGlob}
    if params.IncludeGlob == "*.json" && params.ParentType != "" {
        includeGlobs = defaultDiscoveryGlobsForParent(params.ParentType)
    }
    
    resolver := defaultSpecResolver{}
    resolveReq := ResolveRequest{
        Seeds:             params.Seeds,
        GitHubServiceRoot: params.GitHubRoot,
        DiscoverFromSeed:  params.DiscoverSeeds,
        IncludeGlobs:      includeGlobs,
        IncludePreview:    params.IncludePreview,
        GitHubToken:       githubToken,
    }
    
    resolved, err := resolver.Resolve(context.Background(), resolveReq)
    if err != nil {
        return nil, err
    }
    
    specSources := make([]string, 0, len(resolved.Specs))
    for _, spec := range resolved.Specs {
        if spec.Source != "" {
            specSources = append(specSources, spec.Source)
        }
    }
    
    return specSources, nil
}
```

**Impact:** Eliminates ~80 lines of duplication across 3 command handlers.

---

### 2.3 ToSnakeCase Wrapper Function (LOW PRIORITY)

**Location:** `internal/terraform/hcl_helpers.go`

**Issue:** Unnecessary wrapper function that just calls the imported function:

```go
func toSnakeCase(input string) string {
    return naming.ToSnakeCase(input)
}
```

**Recommendation:**
Remove the `toSnakeCase` wrapper and use `naming.ToSnakeCase` directly throughout the `terraform` package. This is a trivial refactor with no functional impact.

**Implementation:**
1. Search and replace `toSnakeCase(` with `naming.ToSnakeCase(` in `internal/terraform/*.go`
2. Delete the wrapper function from `hcl_helpers.go`
3. Update tests if needed (note: tests currently test the wrapper, should test the actual function)

**Impact:** Removes 3 lines, eliminates unnecessary indirection.

---

## 3. Non-Idiomatic Go Patterns

### 3.1 Excessive Use of `panic()` (HIGH PRIORITY)

**Location:** `internal/terraform/*.go` (8 occurrences)

**Issue:** The codebase uses `panic()` for error conditions that should return errors. This is not idiomatic Go and makes the code harder to test and maintain.

**Evidence:**
```go
// generate_variables.go:37
panic(fmt.Sprintf("failed to get effective properties for array item schema: %v", err))

// generate_variables.go:630
panic(fmt.Sprintf("failed to get effective properties: %v", err))

// secrets.go:89
panic(fmt.Sprintf("failed to get effective properties while scanning for secret fields: %v", err))

// generate_locals.go:57
panic(fmt.Sprintf("failed to get effective properties in constructFlattenedRootPropertiesValue: %v", err))
```

**Problem:** These panics occur in generation code paths that could gracefully return errors. Panics are appropriate for:
- Programmer errors / invariant violations
- Unrecoverable failures during init

They are NOT appropriate for:
- Processing user-provided schemas
- Encountering malformed OpenAPI specs
- Validation failures

**Recommendation:**
Refactor all generation functions to return errors properly:

```go
// Before:
func generateVariables(schema *openapi3.Schema, ...) error {
    props, err := openapi.GetEffectiveProperties(schema)
    if err != nil {
        panic(fmt.Sprintf("failed to get effective properties: %v", err))
    }
    // ...
}

// After:
func generateVariables(schema *openapi3.Schema, ...) error {
    props, err := openapi.GetEffectiveProperties(schema)
    if err != nil {
        return fmt.Errorf("getting effective properties for variable generation: %w", err)
    }
    // ...
}
```

**Impact:** 
- Makes code testable (can test error paths without recovering from panics)
- Provides better user experience (graceful error messages vs crash)
- Follows Go error handling conventions
- Requires updating ~15-20 function signatures to thread errors properly

**Implementation Steps:**
1. Identify all functions with `panic()` calls
2. Change function signatures to return `error` if they don't already
3. Replace `panic(...)` with `return fmt.Errorf(...)`
4. Thread errors up the call chain (some callers will need updates)
5. Update tests to verify error conditions instead of panic recovery

---

### 3.2 Directory Changing Anti-Pattern (MEDIUM PRIORITY)

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

**Recommendation:**
Introduce a `Generator` type that encapsulates the generation context:

```go
type GeneratorContext struct {
    Schema          *openapi3.Schema
    ResourceType    string
    LocalName       string
    APIVersion      string
    NameSchema      *openapi3.Schema
    Spec            *openapi3.T
    OutputDir       string
    
    // Computed/derived fields
    SupportsTags    bool
    SupportsLocation bool
    SupportsIdentity bool
    Secrets         []secretField
    Capabilities    openapi.InterfaceCapabilities
}

type Generator interface {
    Name() string
    Generate(ctx *GeneratorContext) error
}

type Pipeline struct {
    generators []Generator
}

func (p *Pipeline) Run(ctx *GeneratorContext) error {
    for _, gen := range p.generators {
        if err := gen.Generate(ctx); err != nil {
            return fmt.Errorf("%s generation failed: %w", gen.Name(), err)
        }
    }
    return nil
}

// Usage:
pipeline := &Pipeline{
    generators: []Generator{
        &TerraformGenerator{},
        &VariablesGenerator{},
        &LocalsGenerator{},
        &MainGenerator{},
        &OutputsGenerator{},
    },
}

err := pipeline.Run(ctx)
```

**Benefits:**
- Explicit pipeline structure
- Easy to add/remove/reorder generators
- Better error messages (know which generator failed)
- Testable in isolation
- Could support conditional generators (skip if !hasSchema)
- Foundation for plugin system or custom generators

**Impact:**
- Significant refactor (~200 lines changed)
- Better long-term maintainability
- Opens door for future extensibility

---

### 5.2 Centralized Error Types (LOW PRIORITY)

**Location:** Throughout codebase

**Issue:** Error messages are constructed inline everywhere, leading to:
- Inconsistent formatting
- Hard to localize or customize
- Difficult to add error codes
- Can't easily distinguish error types

**Recommendation:**
Define sentinel errors and error types for common cases:

```go
// errors.go
package tfmodmake

import "errors"

var (
    ErrResourceNotFound = errors.New("resource not found in specs")
    ErrSpecLoadFailed   = errors.New("failed to load spec")
    ErrInvalidSpec      = errors.New("invalid OpenAPI specification")
)

type ResourceNotFoundError struct {
    ResourceType string
    SpecsChecked []string
    LoadErrors   []string
    SearchErrors []string
}

func (e *ResourceNotFoundError) Error() string {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("resource type %s not found in any of %d specs", 
        e.ResourceType, len(e.SpecsChecked)))
    if len(e.LoadErrors) > 0 {
        sb.WriteString("\n\nSpec load errors:\n")
        for _, err := range e.LoadErrors {
            sb.WriteString(err)
            sb.WriteString("\n")
        }
    }
    if len(e.SearchErrors) > 0 {
        sb.WriteString("\n\nSpecs checked:\n")
        for _, err := range e.SearchErrors {
            sb.WriteString(err)
            sb.WriteString("\n")
        }
    }
    return sb.String()
}

func (e *ResourceNotFoundError) Unwrap() error {
    return ErrResourceNotFound
}
```

**Impact:**
- Can use `errors.Is()` and `errors.As()` for error handling
- Better error messages with structured information
- Foundation for error codes, localization, etc.

---

## 6. Testing & Quality

### 6.1 Missing Integration Tests (MEDIUM PRIORITY)

**Current State:**
- Good unit test coverage for individual functions
- Integration tests only for specific scenarios (`generator_integration_test.go`, `validations_integration_test.go`)
- No end-to-end CLI command tests

**Recommendation:**
Add integration tests for CLI commands:

```go
// cmd/tfmodmake/integration_test.go
func TestGenCommand_EndToEnd(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantErr  bool
        validate func(t *testing.T, outputDir string)
    }{
        {
            name: "basic generation",
            args: []string{
                "gen",
                "-spec", "testdata/managedClusters.json",
                "-resource", "Microsoft.ContainerService/managedClusters",
            },
            validate: func(t *testing.T, outputDir string) {
                // Check that expected files exist
                assert.FileExists(t, filepath.Join(outputDir, "variables.tf"))
                assert.FileExists(t, filepath.Join(outputDir, "main.tf"))
                assert.FileExists(t, filepath.Join(outputDir, "outputs.tf"))
                // Validate content
                content, err := os.ReadFile(filepath.Join(outputDir, "main.tf"))
                require.NoError(t, err)
                assert.Contains(t, string(content), "azapi_resource")
            },
        },
        // More test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create temp directory
            tmpDir, err := os.MkdirTemp("", "tfmodmake-test-*")
            require.NoError(t, err)
            defer os.RemoveAll(tmpDir)
            
            // Run command
            oldWd, _ := os.Getwd()
            os.Chdir(tmpDir)
            defer os.Chdir(oldWd)
            
            // Execute command programmatically or via subprocess
            // ...
            
            if tt.validate != nil {
                tt.validate(t, tmpDir)
            }
        })
    }
}
```

**Impact:**
- Catch regressions in CLI behavior
- Validate end-to-end workflows
- Document expected usage patterns

---

## 7. Documentation & Naming

### 7.1 Inconsistent Naming Conventions (LOW PRIORITY)

**Issues Found:**
1. Mix of `generate*` and `Generate*` for unexported vs exported functions
2. `stringSliceFlag` type not exported but used across package boundary semantically
3. Some functions prefixed with package name redundantly (e.g., `openapi.LoadSpec` is clear, but `openapi.DiscoverChildren` could be `openapi.Discover`)

**Recommendation:**
- Audit exported vs unexported functions for consistency
- Consider renaming for clarity (e.g., `DiscoverChildren` → `DiscoverChildResources` to be more explicit)
- Document naming conventions in CONTRIBUTING.md

---

## 8. Performance Considerations

### 8.1 Repeated Schema Traversals (LOW PRIORITY)

**Location:** Multiple passes over schemas in generation functions

**Issue:** The codebase makes multiple passes over schemas:
1. First pass: collect secrets
2. Second pass: generate variables
3. Third pass: generate locals
4. Fourth pass: generate outputs

**Recommendation:**
Consider a single-pass analysis that builds a complete intermediate representation:

```go
type SchemaAnalysis struct {
    Properties       map[string]*PropertyInfo
    Secrets          []SecretField
    Computed         []ComputedField
    ValidationsNeeded []ValidationInfo
    SupportsTags     bool
    SupportsLocation bool
    SupportsIdentity bool
}

func AnalyzeSchema(schema *openapi3.Schema) (*SchemaAnalysis, error) {
    // Single pass analysis
    // Collect all information needed for generation
}

// Then generation functions use the pre-analyzed data
```

**Impact:**
- Faster generation for large schemas
- More predictable performance
- Easier to add new analyses without N+1 passes
- ~100 lines of refactoring

---

## 9. Error Handling Improvements

### 9.1 Context Loss in Error Wrapping (LOW PRIORITY)

**Issue:** Some errors lose context when wrapped:

```go
// Before:
if err != nil {
    return fmt.Errorf("failed to generate terraform files: %w", err)
}

// After (with more context):
if err != nil {
    return fmt.Errorf("failed to generate terraform files for resource %s: %w", 
        resourceType, err)
}
```

**Recommendation:**
Audit error messages and ensure they include:
- What operation was being performed
- What resource/file was being processed
- Relevant configuration values

---

## 10. Package Visibility & MCP Server Compatibility (CRITICAL)

### 10.1 Internal Packages Prevent External Reuse (HIGH PRIORITY)

**Location:** All packages under `internal/`

**Issue:** The current package structure places all reusable components under `internal/`, which prevents them from being imported by external code, including MCP (Model Context Protocol) servers.

**Current Structure:**
```
tfmodmake/
  internal/
    hclgen/      - HCL generation utilities
    naming/      - Naming conventions (snake_case, etc.)
    openapi/     - OpenAPI parsing and analysis
    submodule/   - Submodule generation
    terraform/   - Terraform file generation
```

**Problem:** Go's `internal/` directory has special semantics - packages in `internal/` can only be imported by code in the parent tree. This means:
- ❌ Cannot be used by external MCP servers
- ❌ Cannot be used by other projects/tools
- ❌ Cannot create plugins or extensions
- ❌ Limits reusability of well-designed components

**Original Author Requirement:**
> "I don't want the packages to be internal/ if we ever wanted them in an MCP server then they need to be referencable."

### Recommendation: Restructure Package Layout

**New Proposed Structure:**
```
tfmodmake/
  cmd/
    tfmodmake/          - CLI entry point (stays here)
  
  # Public, importable packages
  openapi/              - OpenAPI spec parsing and analysis
    parser.go
    children.go
    allof.go
    writability.go
    capabilities.go
    arm_path.go
  
  naming/               - Naming utilities
    naming.go
  
  terraform/            - Terraform generation
    generator.go
    generate_*.go
    validations.go
    secrets.go
    hcl_helpers.go
  
  hclgen/               - HCL generation utilities
    hclgen.go
  
  submodule/            - Submodule wiring
    generator.go
  
  # Truly internal (CLI-specific, not reusable)
  internal/
    cli/                - CLI-specific helpers
      flags.go          - Flag parsing helpers
      commands.go       - Command routing
      spec_resolver.go  - Spec resolution (GitHub API, etc.)
      spec_discovery.go - GitHub directory discovery
```

### Package Categorization

**Should Be Public (Importable):**

1. **`openapi/`** - Core OpenAPI analysis
   - `LoadSpec()` - Load and parse OpenAPI specs
   - `FindResource()` - Find resource schemas
   - `DiscoverChildren()` - Child resource discovery
   - `GetEffectiveProperties()` - allOf merging
   - `DetectInterfaceCapabilities()` - AVM capability detection
   - **Use Case:** MCP servers need to analyze Azure OpenAPI specs

2. **`naming/`** - Naming conventions
   - `ToSnakeCase()` - Convert to snake_case
   - **Use Case:** Any tool generating Terraform from specs needs consistent naming

3. **`terraform/`** - Terraform generation
   - `Generate()` - Generate complete module
   - `GenerateInterfacesFile()` - Generate AVM interfaces
   - Type mapping, validation generation, etc.
   - **Use Case:** MCP servers generating Terraform code

4. **`hclgen/`** - HCL utilities
   - Token generation helpers
   - HCL file writing utilities
   - **Use Case:** Any tool generating HCL syntax

5. **`submodule/`** - Submodule generation
   - `Generate()` - Create submodule wrappers
   - **Use Case:** Tools creating nested module structures

**Should Stay Internal (CLI-Specific):**

1. **`internal/cli/`** - CLI command handling
   - Flag parsing boilerplate
   - Command routing logic
   - `spec_resolver.go` - Ties together CLI flags + GitHub API
   - `spec_discovery.go` - GitHub-specific discovery
   - **Reason:** Tightly coupled to CLI UX, not reusable

### Migration Plan

#### Pre-Flight Checklist

Before starting, verify:
- [ ] Working directory is clean (`git status`)
- [ ] All tests pass (`go test ./...`)
- [ ] On a feature branch (`git checkout -b refactor/package-restructure`)

#### Step 1: Move Packages (No Code Changes)

**Commands:**
```bash
# From repository root: /Users/stumace/src/matt-FFFFFF/tfmodmake

# Move packages out of internal/ (preserves git history)
git mv internal/openapi openapi
git mv internal/naming naming
git mv internal/terraform terraform
git mv internal/hclgen hclgen
git mv internal/submodule submodule

# Create new internal/cli for CLI-specific code
mkdir -p internal/cli

# Verify structure
ls -la | grep -E "(openapi|naming|terraform|hclgen|submodule|internal)"
```

**Verification:**
```bash
# Should see:
# drwxr-xr-x  openapi/
# drwxr-xr-x  naming/
# drwxr-xr-x  terraform/
# drwxr-xr-x  hclgen/
# drwxr-xr-x  submodule/
# drwxr-xr-x  internal/

# internal/ should now only contain .gitkeep or be empty except for cli/
```

#### Step 2: Extract CLI-Specific Code

**File Moves:**
```bash
# Move spec resolution logic to internal/cli
git mv cmd/tfmodmake/spec_resolver.go internal/cli/spec_resolver.go
git mv cmd/tfmodmake/spec_discovery.go internal/cli/spec_discovery.go

# Move corresponding tests
git mv cmd/tfmodmake/spec_resolver_test.go internal/cli/spec_resolver_test.go
git mv cmd/tfmodmake/spec_discovery_test.go internal/cli/spec_discovery_test.go
```

**Files Remaining in cmd/tfmodmake/:**
- `main.go` - Main entry point and command routing
- `addchild_test.go` - Command-level integration test (uses internal/cli)

**Create internal/cli/package.go:**
```bash
cat > internal/cli/package.go << 'EOF'
// Package cli contains CLI-specific helpers for the tfmodmake command-line tool.
//
// This package is intentionally internal and not meant to be imported by
// external tools or MCP servers. For reusable functionality, see the public
// packages: openapi, terraform, naming, hclgen, and submodule.
package cli
EOF
```

#### Step 3: Update Import Paths

**Complete File Inventory for Import Updates:**

Files in `cmd/tfmodmake/`:
- [x] `main.go` - imports: internal/naming, internal/openapi, internal/submodule, internal/terraform
- [x] `addchild_test.go` - imports: internal/openapi, internal/terraform

Files moved to `internal/cli/`:
- [x] `spec_resolver.go` - imports: none (self-contained)
- [x] `spec_discovery.go` - imports: none (self-contained)
- [x] `spec_resolver_test.go` - imports: none
- [x] `spec_discovery_test.go` - imports: none

Public package files (cross-imports between public packages):
- [x] `openapi/*.go` - may import: naming (check all files)
- [x] `terraform/*.go` - imports: openapi, naming, hclgen (check all files)
- [x] `terraform/*_test.go` - imports: openapi (check all test files)
- [x] `hclgen/*.go` - imports: none
- [x] `submodule/*.go` - may import: terraform, naming (check)
- [x] `naming/*.go` - imports: none

**Automated Import Update Commands:**

```bash
# Update cmd/tfmodmake/main.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/naming|github.com/matt-FFFFFF/tfmodmake/naming|g' cmd/tfmodmake/main.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/openapi|github.com/matt-FFFFFF/tfmodmake/openapi|g' cmd/tfmodmake/main.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/submodule|github.com/matt-FFFFFF/tfmodmake/submodule|g' cmd/tfmodmake/main.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/terraform|github.com/matt-FFFFFF/tfmodmake/terraform|g' cmd/tfmodmake/main.go

# Update cmd/tfmodmake/addchild_test.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/openapi|github.com/matt-FFFFFF/tfmodmake/openapi|g' cmd/tfmodmake/addchild_test.go
sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/terraform|github.com/matt-FFFFFF/tfmodmake/terraform|g' cmd/tfmodmake/addchild_test.go

# Update all files in openapi/
find openapi -name '*.go' -exec sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/naming|github.com/matt-FFFFFF/tfmodmake/naming|g' {} +

# Update all files in terraform/
find terraform -name '*.go' -exec sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/openapi|github.com/matt-FFFFFF/tfmodmake/openapi|g' {} +
find terraform -name '*.go' -exec sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/naming|github.com/matt-FFFFFF/tfmodmake/naming|g' {} +
find terraform -name '*.go' -exec sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/hclgen|github.com/matt-FFFFFF/tfmodmake/hclgen|g' {} +

# Update all files in submodule/
find submodule -name '*.go' -exec sed -i '' 's|github.com/matt-FFFFFF/tfmodmake/internal/|github.com/matt-FFFFFF/tfmodmake/|g' {} +

# Run goimports to clean up and organize imports
go install golang.org/x/tools/cmd/goimports@latest
find . -name '*.go' -not -path './vendor/*' -exec goimports -w {} +
```

**Manual Verification:**
```bash
# Check for any remaining internal/ imports in public packages
grep -r "internal/" openapi/*.go terraform/*.go naming/*.go hclgen/*.go submodule/*.go

# Should return NO results. If it does, those need manual fixing.

# Check cmd/tfmodmake imports are updated
grep "import" cmd/tfmodmake/main.go | grep -E "(openapi|terraform|naming|submodule)"
# Should show new paths without internal/
```

**Add internal/cli imports to main.go:**
```bash
# main.go will need to import internal/cli for spec resolution
# Add this import manually or via editor:
# import "github.com/matt-FFFFFF/tfmodmake/internal/cli"
```

Then update `main.go` to use `cli.` prefix for:
- `ResolveRequest` → `cli.ResolveRequest`
- `ResolveResult` → `cli.ResolveResult`
- `ResolvedSpec` → `cli.ResolvedSpec`
- `defaultSpecResolver` → `cli.defaultSpecResolver`
- `parseGitHubTreeDirURL` → `cli.parseGitHubTreeDirURL`
- `parseRawGitHubFileURL` → `cli.parseRawGitHubFileURL`
- `listGitHubDirectoryDownloadURLs` → `cli.listGitHubDirectoryDownloadURLs`
- `discoverSiblingSpecsFromRawGitHubSpecURL` → `cli.discoverSiblingSpecsFromRawGitHubSpecURL`
- `discoverDeterministicSpecSetFromGitHubDir` → `cli.discoverDeterministicSpecSetFromGitHubDir`
- `deterministicDiscoveryOptions` → `cli.deterministicDiscoveryOptions`
- `githubTokenFromEnv` → `cli.githubTokenFromEnv`
- `defaultDiscoveryGlobsForParent` → `cli.defaultDiscoveryGlobsForParent`
- `writeResolvedSpecs` → `cli.writeResolvedSpecs`

**Search Pattern for Types/Functions to Prefix:**
```bash
# Find all type and function references that moved to internal/cli
grep -E "(type|func) (Resolve|defaultSpec|parseGitHub|listGitHub|discover|githubToken|writeResolved)" cmd/tfmodmake/spec_*.go
```

#### Step 4: Update Package Declarations in Moved Files

**Update internal/cli files:**
```bash
# Update package declaration in moved files
sed -i '' 's/^package main$/package cli/' internal/cli/spec_resolver.go
sed -i '' 's/^package main$/package cli/' internal/cli/spec_discovery.go
sed -i '' 's/^package main$/package cli/' internal/cli/spec_resolver_test.go
sed -i '' 's/^package main$/package cli/' internal/cli/spec_discovery_test.go

# Make types/functions exported (capitalize first letter) in internal/cli files
# This is manual - look for unexported types like:
# type resolvedSpec → type ResolvedSpec
# type resolveRequest → type ResolveRequest
# func githubTokenFromEnv → func GithubTokenFromEnv
# etc.
```

**Note:** Functions that are truly internal helpers can remain unexported.

#### Step 5: Add Package Documentation

**For each public package, add doc.go:**

```bash
# openapi/doc.go
cat > openapi/doc.go << 'EOF'
// Package openapi provides utilities for parsing and analyzing Azure OpenAPI specifications.
//
// This package is designed to be importable by external tools and MCP servers that need
// to work with Azure REST API specs.
//
// # Key Functions
//
// LoadSpec loads an OpenAPI specification from a file path or URL:
//
//	doc, err := openapi.LoadSpec("https://raw.githubusercontent.com/.../spec.json")
//
// FindResource locates a specific ARM resource type within a spec:
//
//	schema, err := openapi.FindResource(doc, "Microsoft.ContainerService/managedClusters")
//
// DiscoverChildren finds deployable child resources under a parent:
//
//	opts := openapi.DiscoverChildrenOptions{
//	    Specs:  []string{"spec1.json", "spec2.json"},
//	    Parent: "Microsoft.App/managedEnvironments",
//	    Depth:  1,
//	}
//	result, err := openapi.DiscoverChildren(opts)
//
// GetEffectiveProperties resolves allOf inheritance to get the complete property set:
//
//	props, err := openapi.GetEffectiveProperties(schema)
//
// # Schema Analysis
//
// The package provides utilities for analyzing OpenAPI schemas including:
//   - allOf/oneOf/anyOf composition handling
//   - Read-only vs writable property detection
//   - Azure-specific extensions (x-ms-secret, x-ms-mutability)
//   - ARM path parsing and validation
//
// # Compatibility
//
// This package handles both OpenAPI 3.0 and Swagger 2.0 specs as used by
// Azure REST API specifications in the Azure/azure-rest-api-specs repository.
package openapi
EOF

# naming/doc.go
cat > naming/doc.go << 'EOF'
// Package naming provides naming convention utilities for code generation.
//
// The primary export is ToSnakeCase which converts various naming styles
// (camelCase, PascalCase, kebab-case, etc.) into snake_case following
// Go and Terraform naming conventions.
//
// # Example
//
//	snakeName := naming.ToSnakeCase("HTTPSEndpoint")
//	// Result: "https_endpoint"
//
//	snakeName := naming.ToSnakeCase("io.k8s.api.core.v1.PodSpec")
//	// Result: "io_k8s_api_core_v1_pod_spec"
//
// # Handling
//
// The function handles:
//   - Acronyms (HTTP, HTTPS, API → http, https, api)
//   - Numbers (starting with digit → prefixed with "field_")
//   - Special characters (replaced with underscores)
//   - Consecutive uppercase letters (HTTPServer → http_server)
package naming
EOF

# terraform/doc.go
cat > terraform/doc.go << 'EOF'
// Package terraform provides code generation for Terraform modules from OpenAPI schemas.
//
// This package generates complete Terraform module structures including:
//   - variables.tf - Input variable definitions with types and validations
//   - locals.tf - Local value transformations
//   - main.tf - Resource definitions (azapi_resource)
//   - outputs.tf - Computed output values
//   - terraform.tf - Provider requirements
//
// Optionally, it can generate:
//   - main.interfaces.tf - AVM interfaces scaffolding
//
// # Basic Usage
//
//	err := terraform.Generate(
//	    schema,              // *openapi3.Schema
//	    "Microsoft.ContainerService/managedClusters",
//	    "resource_body",     // local variable name
//	    "2024-01-01",       // API version
//	    true,               // supports tags
//	    true,               // supports location
//	    nameSchema,         // *openapi3.Schema for name validation
//	    spec,               // *openapi3.T for capability detection
//	)
//
// # Validation Generation
//
// The package automatically generates Terraform validation blocks from OpenAPI constraints:
//   - String: minLength, maxLength, pattern, format, enum
//   - Number: minimum, maximum, multipleOf
//   - Array: minItems, maxItems, uniqueItems
//
// # Secret Handling
//
// Fields marked with x-ms-secret or writeOnly are handled specially:
//   - Generated as ephemeral variables
//   - Excluded from body
//   - Added to sensitive_body with version mapping
//
// # AVM Interfaces
//
// For Azure Verified Modules, use GenerateInterfacesFile to scaffold common
// AVM interfaces (role assignments, locks, private endpoints, etc.):
//
//	err := terraform.GenerateInterfacesFile(resourceType, spec)
package terraform
EOF

# hclgen/doc.go
cat > hclgen/doc.go << 'EOF'
// Package hclgen provides utilities for generating HCL (HashiCorp Configuration Language) code.
//
// This package offers helpers for constructing HCL tokens, expressions, and files
// programmatically without string templates.
//
// # Token Builders
//
// TokensForTraversal builds dot-separated identifier paths:
//
//	tokens := hclgen.TokensForTraversal("var", "network_profile", "subnet_id")
//	// Produces: var.network_profile.subnet_id
//
// TokensForHeredoc creates multi-line string literals:
//
//	tokens := hclgen.TokensForHeredoc("This is a\nmulti-line description")
//	// Produces: <<DESCRIPTION\nThis is a\nmulti-line description\nDESCRIPTION
//
// NullEqualityTernary builds null-safe ternary expressions:
//
//	tokens := hclgen.NullEqualityTernary(
//	    hclgen.TokensForTraversal("var", "optional_value"),
//	    hclgen.TokensForTraversal("local", "computed_value"),
//	)
//	// Produces: var.optional_value == null ? null : local.computed_value
//
// # File Writing
//
// WriteFile writes an HCL file to disk with proper formatting:
//
//	file := hclwrite.NewEmptyFile()
//	// ... build file content ...
//	err := hclgen.WriteFile("main.tf", file)
package hclgen
EOF

# submodule/doc.go
cat > submodule/doc.go << 'EOF'
// Package submodule provides utilities for generating Terraform submodule wrappers.
//
// This package creates "wrapper" files that integrate child modules into a parent
// module using for_each patterns for map-based configuration.
//
// # Usage
//
// Given an existing child module at modules/certificates/, Generate will create:
//   - variables.certificates.tf - Map variable accepting certificate configs
//   - main.certificates.tf - for_each module block invoking the child module
//
// Example:
//
//	err := submodule.Generate("modules/certificates")
//
// # Generated Pattern
//
// The generated code follows this pattern:
//
//	# variables.certificates.tf
//	variable "certificates" {
//	  type = map(object({
//	    # ... inferred from child module variables ...
//	  }))
//	  default = {}
//	}
//
//	# main.certificates.tf
//	module "certificates" {
//	  source   = "./modules/certificates"
//	  for_each = var.certificates
//
//	  # Pass through variables
//	  name      = each.key
//	  parent_id = azapi_resource.this.id
//	  # ... other variables ...
//	}
//
// This allows users to define multiple child resources declaratively via a map variable.
package submodule
EOF
```

#### Step 6: Run Tests and Fix Issues

```bash
# Clean go module cache
go clean -modcache

# Tidy dependencies
go mod tidy

# Run all tests
go test ./...

# Expected output: PASS for all packages
# If failures occur, fix import issues

# Run integration tests
go test -v ./cmd/tfmodmake/...

# Build binary to ensure no linking issues
go build -o tfmodmake ./cmd/tfmodmake

# Test the binary
./tfmodmake --help
```

**Common Issues and Fixes:**

1. **"package internal/xyz is not in GOROOT"**
   - Missed an import update
   - Search: `grep -r "internal/" --include="*.go"`

2. **"undefined: SomeType"**
   - Type not exported from internal/cli
   - Either export it or keep it in cmd/tfmodmake

3. **Circular import**
   - Public package importing internal/cli
   - Move shared code to public package

4. **Test failures**
   - Update test imports
   - Fix test data paths if any

#### Step 7: Verify No Internal Imports in Public Packages

```bash
# Critical check: public packages should not import internal/
for pkg in openapi naming terraform hclgen submodule; do
    echo "Checking $pkg..."
    if grep -r "internal/" $pkg/*.go 2>/dev/null; then
        echo "ERROR: $pkg imports internal/ - must fix!"
        exit 1
    fi
done

echo "✓ All public packages are clean"
```

#### Step 8: External Import Test

Create a test outside the repo:

```bash
# In a temp directory
cd /tmp
mkdir tfmodmake-import-test
cd tfmodmake-import-test

cat > go.mod << 'EOF'
module test

go 1.22

require github.com/matt-FFFFFF/tfmodmake v0.0.0
EOF

cat > main.go << 'EOF'
package main

import (
    "fmt"
    "github.com/matt-FFFFFF/tfmodmake/openapi"
    "github.com/matt-FFFFFF/tfmodmake/naming"
)

func main() {
    // Test that packages are importable
    fmt.Println("Testing package imports...")
    
    // Test naming
    result := naming.ToSnakeCase("HTTPSEndpoint")
    fmt.Printf("ToSnakeCase: %s\n", result)
    
    // Test openapi (would need a real spec)
    fmt.Println("openapi.LoadSpec is available:", openapi.LoadSpec != nil)
    
    fmt.Println("✓ All imports successful")
}
EOF

# Point to local repo for testing
go mod edit -replace github.com/matt-FFFFFF/tfmodmake=/Users/stumace/src/matt-FFFFFF/tfmodmake

# This should work now
go run main.go
```

#### Step 9: Commit Changes

```bash
# Review changes
git status
git diff --cached

# Commit the restructure
git add -A
git commit -m "refactor: move packages out of internal/ for external reusability

- Move openapi, naming, terraform, hclgen, submodule to public packages
- Create internal/cli for CLI-specific helpers (spec resolution, GitHub discovery)
- Update all import paths throughout codebase
- Add package-level documentation for public APIs
- Verify no internal/ imports in public packages

This enables MCP servers and external tools to import and use tfmodmake packages
while keeping CLI-specific logic properly internal.

Relates to: code review remediation plan Phase 0"

# Push to branch
git push origin refactor/package-restructure
```

#### Rollback Procedure (if needed)

```bash
# If something goes wrong, rollback:
git reset --hard HEAD~1

# Or if already pushed:
git revert HEAD
git push origin refactor/package-restructure
```

### Benefits

1. **MCP Server Integration** ✓
   - MCP servers can import and use the packages
   - Can build extensions/plugins
   
2. **Code Reusability** ✓
   - Other tools can leverage the OpenAPI parsing
   - Terraform generation can be embedded in other projects
   
3. **Better Testing** ✓
   - External consumers validate API design
   - More eyes on the code
   
4. **Community Contributions** ✓
   - Easier for others to build on top of tfmodmake
   - Clear separation of public vs internal APIs

### Risks

1. **API Stability Burden**
   - Need to maintain backward compatibility
   - Breaking changes require major version bumps
   - **Mitigation:** Start with clear documentation, mark experimental APIs

2. **Accidental Coupling**
   - Public packages might inadvertently depend on internal ones
   - **Mitigation:** Use linting to detect `internal/` imports in public packages

3. **Documentation Overhead**
   - Public APIs need better documentation
   - **Mitigation:** Add godoc comments incrementally

### Testing Strategy

Covered in Step 6-8 of Migration Plan above.

---

## Implementation Roadmap

### Phase 0: Package Restructuring (1-2 days) **[NEW - HIGHEST PRIORITY]**
1. Move packages out of `internal/` ✓
2. Create `internal/cli/` for CLI-specific code ✓
3. Update all import paths ✓
4. Add package-level documentation ✓
5. Verify no `internal/` imports in public packages ✓
6. Test external import scenario ✓

**Estimated LOC Impact:** Neutral (restructuring only)

### Phase 1: Quick Wins (1-2 days)
1. Remove legacy command handling ✓
2. Remove `toSnakeCase` wrapper ✓
3. Add common flag definition helpers ✓
4. Extract spec resolution helper ✓

**Estimated LOC Impact:** -150 lines

### Phase 2: Error Handling (3-5 days)
1. Replace panics with error returns ✓
2. Add custom error types ✓
3. Improve error context ✓
4. Update tests for error paths ✓

**Estimated LOC Impact:** +100 lines (better quality)

### Phase 3: Code Consolidation (3-5 days)
1. Extract `loadResourceFromSpecs` helper ✓
2. Consolidate spec resolution ✓
3. Improve flag validation ✓

**Estimated LOC Impact:** -200 lines

### Phase 4: Architectural Improvements (5-10 days)
1. Remove directory changing anti-pattern ✓
2. Implement generator pipeline ✓
3. Single-pass schema analysis (optional) ✓

**Estimated LOC Impact:** +50 lines (better structure)

### Phase 5: Testing & Documentation (3-5 days)
1. Add CLI integration tests ✓
2. Document naming conventions ✓
3. Add performance benchmarks (optional) ✓

---

## Metrics Summary

| Metric | Current | After Refactor | Change |
|--------|---------|----------------|--------|
| Public Packages | 0 | 5 | +5 |
| Internal Packages | 5 | 1 | -4 |
| Lines of Code (cmd/tfmodmake) | ~960 | ~750 | -210 |
| Duplicate Code Blocks | 5 | 0 | -5 |
| Panic Calls | 8 | 0 | -8 |
| Test Coverage (estimate) | 70% | 80% | +10% |
| Cyclomatic Complexity (main.go) | High | Medium | Improved |
| External Reusability | No | Yes | ✓ |

---

## Risk Assessment

### Low Risk
- **Moving packages out of internal/** (mechanical refactor, no logic changes)

### Medium Risk
- Replacing panics with errors (requires testing all paths)
- Consolidating duplicate code (need to verify equivalent behavior)
- Flag parsing changes (must maintain CLI compatibility)
- **Maintaining API stability** (new responsibility for public packages
- Replacing panics with errors (requires testing all paths)
- Consolidating duplicate code (need to verify equivalent behavior)
- Flag parsing changes (must maintain CLI compatibility)

### High Risk
- Generator pipeline refactor (large structural change)
- Directory changing removal (touches many files)
- Single-pass analysis (fundamental algorithm change)

**Recommendation:** Implement in phases, with thorough testing after each phase.

---

## Conclusion

The tEnable external reusability** by moving packages out of `internal/` (CRITICAL for MCP server integration)
2. **Reduce code size** by ~15% through deduplication
3. **Improve error handling** to be more idiomatic Go
4. **Simplify CLI handling** for better UX and maintainability
5. **Establish patterns** for future feature additions

**Priority Order:**
1. **Restructure packages** (enables MCP server usage - HIGHEST PRIORITY per author requirement)
2. Remove legacy commands (immediate cleanup)
3. Fix panic usage (correctness issue)
4. Extract duplicate patterns (maintenance burden)
5. Consider architectural improvements (future-proofing)

All changes should maintain external compatibility (OpenAPI inputs, Terraform outputs, documented CLI commands).

**Critical Note:** Phase 0 (package restructuring) must be completed before other phases, as it changes import paths throughout the codebase. However, it's a low-risk mechanical refactor that unlocks the tool's future as a reusable library

All changes should maintain external compatibility (OpenAPI inputs, Terraform outputs, documented CLI commands).
