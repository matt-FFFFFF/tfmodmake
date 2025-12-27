package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/matt-FFFFFF/tfmodmake/internal/naming"
	"github.com/matt-FFFFFF/tfmodmake/internal/openapi"
	"github.com/matt-FFFFFF/tfmodmake/internal/submodule"
	"github.com/matt-FFFFFF/tfmodmake/internal/terraform"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "addsub" {
		addSub := flag.NewFlagSet("addsub", flag.ExitOnError)
		if err := addSub.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse addsub arguments: %v", err)
		}
		args := addSub.Args()
		if len(args) != 1 {
			log.Fatalf("Usage: %s addsub <path>", os.Args[0])
		}
		if err := submodule.Generate(args[0]); err != nil {
			log.Fatalf("Failed to add submodule: %v", err)
		}
		fmt.Println("Successfully generated submodule wrapper files")
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "addchild" {
		handleAddChildCommand()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "children" {
		handleChildrenCommand()
		return
	}

	specPath := flag.String("spec", "", "Path or URL to the OpenAPI specification")
	resourceType := flag.String("resource", "", "Resource type to generate Terraform configuration for (e.g. Microsoft.ContainerService/managedClusters)")
	rootPath := flag.String("root", "", "Path to the root object (e.g. properties or properties.foo)")
	localName := flag.String("local-name", "", "Name of the local variable to generate (default: resource_body or derived from root)")
	flag.Parse()

	if *specPath == "" || *resourceType == "" {
		flag.Usage()
		os.Exit(1)
	}

	doc, err := openapi.LoadSpec(*specPath)
	if err != nil {
		log.Fatalf("Failed to load spec: %v", err)
	}

	apiVersion := ""
	if doc.Info != nil {
		apiVersion = doc.Info.Version
	}

	schema, err := openapi.FindResource(doc, *resourceType)
	if err != nil {
		log.Fatalf("Failed to find resource: %v", err)
	}

	nameSchema, err := openapi.FindResourceNameSchema(doc, *resourceType)
	if err != nil {
		log.Fatalf("Failed to find resource name schema: %v", err)
	}

	// Some Azure specs illegally combine `$ref` with sibling metadata like `readOnly`.
	// Many parsers drop those siblings when resolving refs, so we re-apply property
	// writability from the raw spec JSON where possible.
	openapi.AnnotateSchemaRefOrigins(schema)
	if resolver, err := openapi.NewPropertyWritabilityResolver(*specPath); err == nil && resolver != nil {
		openapi.ApplyPropertyWritabilityOverrides(schema, resolver)
	}

	supportsTags := terraform.SupportsTags(schema)
	supportsLocation := terraform.SupportsLocation(schema)

	if *rootPath != "" {
		schema, err = openapi.NavigateSchema(schema, *rootPath)
		if err != nil {
			log.Fatalf("Failed to navigate to root path %s: %v", *rootPath, err)
		}
	}

	finalLocalName := "resource_body"
	if *localName != "" {
		finalLocalName = *localName
	} else if *rootPath != "" {
		// properties.networkProfile -> properties_network_profile
		finalLocalName = strings.ReplaceAll(*rootPath, ".", "_")
		finalLocalName = naming.ToSnakeCase(finalLocalName)
	}

	if err := terraform.Generate(schema, *resourceType, finalLocalName, apiVersion, supportsTags, supportsLocation, nameSchema); err != nil {
		log.Fatalf("Failed to generate terraform files: %v", err)
	}

	fmt.Println("Successfully generated Terraform files")
}

// stringSliceFlag is a custom flag type that allows multiple values.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func handleChildrenCommand() {
	childrenCmd := flag.NewFlagSet("children", flag.ExitOnError)

	var specs stringSliceFlag
	childrenCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
	specRoot := childrenCmd.String("spec-root", "", "GitHub tree URL under Azure/azure-rest-api-specs (https://github.com/Azure/azure-rest-api-specs/tree/<ref>/<dir>)")
	discoverFromSpec := childrenCmd.Bool("discover", false, "Discover additional spec files from the directory of the provided raw.githubusercontent.com -spec URL")
	includePreview := childrenCmd.Bool("include-preview", false, "When discovering specs from a GitHub service root, also include the latest preview API version folder")
	includeGlob := childrenCmd.String("include", "*.json", "Glob filter used when discovering spec files (matched against filename, e.g. ManagedEnvironments*.json)")
	parent := childrenCmd.String("parent", "", "Parent resource type (e.g. Microsoft.App/managedEnvironments)")
	jsonOutput := childrenCmd.Bool("json", false, "Output results as JSON instead of markdown")
	printResolvedSpecs := childrenCmd.Bool("print-resolved-specs", false, "Print the resolved spec list to stderr before analysis")

	if err := childrenCmd.Parse(os.Args[2:]); err != nil {
		log.Fatalf("Failed to parse children arguments: %v", err)
	}

	githubToken := githubTokenFromEnv()

	includeGlobs := []string{*includeGlob}
	if *includeGlob == "*.json" && *parent != "" {
		includeGlobs = defaultDiscoveryGlobsForParent(*parent)
	}

	resolver := defaultSpecResolver{}
	resolveReq := ResolveRequest{
		Seeds:            specs,
		GitHubServiceRoot: *specRoot,
		DiscoverFromSeed:  *discoverFromSpec,
		IncludeGlobs:      includeGlobs,
		IncludePreview:    *includePreview,
		GitHubToken:       githubToken,
	}
	resolved, err := resolver.Resolve(context.Background(), resolveReq)
	if err != nil {
		log.Fatalf("Failed to resolve specs: %v", err)
	}

	if *printResolvedSpecs {
		writeResolvedSpecs(os.Stderr, resolved.Specs)
	}

	// Extract sources for analysis (keep the flag-backed "specs" slice unmodified).
	specSources := make([]string, 0, len(resolved.Specs))
	for _, spec := range resolved.Specs {
		if spec.Source == "" {
			continue
		}
		specSources = append(specSources, spec.Source)
	}

	if len(specSources) == 0 {
		log.Fatalf("Usage: %s children -spec <path_or_url> [-discover] [-include-preview] [-include <glob>] [-spec-root <url>] -parent <resource_type> [-json]\nAt least one -spec is required (or use -spec-root / -discover to expand specs)", os.Args[0])
	}

	if *parent == "" {
		log.Fatalf("Usage: %s children -spec <path_or_url> -parent <resource_type> [-json]\n-parent is required", os.Args[0])
	}

	opts := openapi.DiscoverChildrenOptions{
		Specs:  specSources,
		Parent: *parent,
		Depth:  1, // Direct children only
	}

	result, err := openapi.DiscoverChildren(opts)
	if err != nil {
		log.Fatalf("Failed to discover children: %v", err)
	}

	if *jsonOutput {
		jsonStr, err := openapi.FormatChildrenAsJSON(result)
		if err != nil {
			log.Fatalf("Failed to format as JSON: %v", err)
		}
		fmt.Println(jsonStr)
	} else {
		text := openapi.FormatChildrenAsText(result)
		fmt.Print(text)
	}
}

func handleAddChildCommand() {
	addChildCmd := flag.NewFlagSet("addchild", flag.ExitOnError)

	var specs stringSliceFlag
	addChildCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
	specRoot := addChildCmd.String("spec-root", "", "GitHub tree URL under Azure/azure-rest-api-specs (https://github.com/Azure/azure-rest-api-specs/tree/<ref>/<dir>)")
	includePreview := addChildCmd.Bool("include-preview", false, "When discovering specs from a GitHub service root, also include the latest preview API version folder")
	includeGlob := addChildCmd.String("include", "*.json", "Glob filter used when discovering spec files (matched against filename)")
	parent := addChildCmd.String("parent", "", "Parent resource type (e.g. Microsoft.App/managedEnvironments)")
	child := addChildCmd.String("child", "", "Child resource type (e.g. Microsoft.App/managedEnvironments/storages)")
	moduleDir := addChildCmd.String("module-dir", "modules", "Directory where child modules live")
	moduleName := addChildCmd.String("module-name", "", "Override derived module folder name")
	dryRun := addChildCmd.Bool("dry-run", false, "Print planned actions without writing files")

	if err := addChildCmd.Parse(os.Args[2:]); err != nil {
		log.Fatalf("Failed to parse addchild arguments: %v", err)
	}

	const addChildUsage = "Usage: %s addchild -parent <resource_type> -child <resource_type> [-spec <path_or_url>] [-spec-root <url>] [-module-dir <path>] [-module-name <name>] [-dry-run]"

	if *parent == "" {
		log.Fatalf(addChildUsage+"\n-parent is required", os.Args[0])
	}

	if *child == "" {
		log.Fatalf(addChildUsage+"\n-child is required", os.Args[0])
	}

	if len(specs) == 0 && *specRoot == "" {
		log.Fatalf(addChildUsage+"\nAt least one -spec or -spec-root is required", os.Args[0])
	}

	githubToken := githubTokenFromEnv()

	includeGlobs := []string{*includeGlob}
	if *includeGlob == "*.json" && *parent != "" {
		includeGlobs = defaultDiscoveryGlobsForParent(*parent)
	}

	// Resolve specs using the same logic as children command
	resolver := defaultSpecResolver{}
	resolveReq := ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: *specRoot,
		DiscoverFromSeed:  false, // Not needed for addchild
		IncludeGlobs:      includeGlobs,
		IncludePreview:    *includePreview,
		GitHubToken:       githubToken,
	}
	resolved, err := resolver.Resolve(context.Background(), resolveReq)
	if err != nil {
		log.Fatalf("Failed to resolve specs: %v", err)
	}

	specSources := make([]string, 0, len(resolved.Specs))
	for _, spec := range resolved.Specs {
		if spec.Source == "" {
			continue
		}
		specSources = append(specSources, spec.Source)
	}

	if len(specSources) == 0 {
		log.Fatalf("No specs resolved. Please provide -spec or -spec-root.")
	}

	// Derive module name from child type if not provided
	finalModuleName := *moduleName
	if finalModuleName == "" {
		finalModuleName = deriveModuleName(*child)
	}

	// Construct module path using filepath.Join for portability
	modulePath := filepath.Join(*moduleDir, finalModuleName)

	if *dryRun {
		fmt.Printf("DRY RUN: Would create/update child module at: %s\n", modulePath)
		fmt.Printf("DRY RUN: Would generate the following files in child module:\n")
		fmt.Printf("  - %s\n", filepath.Join(modulePath, "variables.tf"))
		fmt.Printf("  - %s\n", filepath.Join(modulePath, "locals.tf"))
		fmt.Printf("  - %s\n", filepath.Join(modulePath, "main.tf"))
		fmt.Printf("  - %s\n", filepath.Join(modulePath, "outputs.tf"))
		fmt.Printf("  - %s\n", filepath.Join(modulePath, "terraform.tf"))
		fmt.Printf("DRY RUN: Would wire child module into root module with:\n")
		moduleDirName := filepath.Base(modulePath)
		fmt.Printf("  - variables.%s.tf\n", moduleDirName)
		fmt.Printf("  - main.%s.tf\n", moduleDirName)
		fmt.Printf("DRY RUN: Using %d resolved spec(s)\n", len(specSources))
		return
	}

	// Step 1: Generate/scaffold the child module
	if err := generateChildModule(specSources, *child, modulePath); err != nil {
		log.Fatalf("Failed to generate child module: %v", err)
	}

	// Step 2: Wire the child module into the root module using addsub logic
	if err := submodule.Generate(modulePath); err != nil {
		log.Fatalf("Failed to wire child module: %v", err)
	}

	fmt.Printf("Successfully created child module at: %s\n", modulePath)
	fmt.Println("Successfully generated submodule wrapper files")
}

// deriveModuleName derives a module folder name from a child resource type.
// Example: "Microsoft.App/managedEnvironments/storages" -> "storages"
func deriveModuleName(childType string) string {
	// Remove any trailing placeholder
	normalized := childType
	if strings.HasSuffix(normalized, "}") {
		if idx := strings.LastIndex(normalized, "/{"); idx != -1 {
			normalized = normalized[:idx]
		}
	}

	// Get the last segment
	lastSlash := strings.LastIndex(normalized, "/")
	if lastSlash == -1 {
		return normalized
	}

	return normalized[lastSlash+1:]
}

// generateChildModule generates a child module scaffold at the specified path.
func generateChildModule(specs []string, childType, modulePath string) error {
	// Create module directory if it doesn't exist
	if err := os.MkdirAll(modulePath, 0o755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	// Load specs and find the child resource
	var schema *openapi3.Schema
	var nameSchema *openapi3.Schema
	var apiVersion string
	var supportsTags bool
	var supportsLocation bool

	var loadErrors []string
	var searchErrors []string

	for _, specPath := range specs {
		doc, err := openapi.LoadSpec(specPath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue // Try next spec
		}

		// Try to find the child resource in this spec
		foundSchema, err := openapi.FindResource(doc, childType)
		if err != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue // Try next spec
		}

		// Found the resource!
		schema = foundSchema

		// Get API version
		if doc.Info != nil {
			apiVersion = doc.Info.Version
		}

		// Get name schema
		nameSchema, _ = openapi.FindResourceNameSchema(doc, childType)

		// Apply writability overrides
		openapi.AnnotateSchemaRefOrigins(schema)
		if resolver, err := openapi.NewPropertyWritabilityResolver(specPath); err == nil && resolver != nil {
			openapi.ApplyPropertyWritabilityOverrides(schema, resolver)
		}

		// Check for tags and location support
		supportsTags = terraform.SupportsTags(schema)
		supportsLocation = terraform.SupportsLocation(schema)

		break
	}

	if schema == nil {
		errMsg := fmt.Sprintf("child resource type %s not found in any of the provided specs", childType)
		if len(loadErrors) > 0 {
			errMsg += fmt.Sprintf("\n\nSpec load errors:\n%s", strings.Join(loadErrors, "\n"))
		}
		if len(searchErrors) > 0 {
			errMsg += fmt.Sprintf("\n\nSpecs checked:\n%s", strings.Join(searchErrors, "\n"))
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Generate Terraform files in the module directory
	// We need to temporarily change directory because terraform.Generate writes to the current directory
	if err := generateInDirectory(modulePath, func() error {
		localName := "resource_body"
		return terraform.Generate(schema, childType, localName, apiVersion, supportsTags, supportsLocation, nameSchema)
	}); err != nil {
		return fmt.Errorf("failed to generate terraform files: %w", err)
	}

	return nil
}

// generateInDirectory changes to the specified directory, executes the function, and changes back.
// This is a temporary workaround until terraform.Generate can accept an output directory parameter.
func generateInDirectory(dir string, fn func() error) error {
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("failed to change to directory %s: %w", dir, err)
	}

	// Ensure we change back even if fn panics
	defer func() {
		if chErr := os.Chdir(originalDir); chErr != nil {
			// Log the error but don't override the original error
			log.Printf("Warning: failed to restore directory to %s: %v", originalDir, chErr)
		}
	}()

	return fn()
}

func defaultDiscoveryGlobsForParent(parentType string) []string {
	// When the user didn't specify -include (defaults to *.json), try a narrower
	// pattern first to avoid pulling unrelated specs from big version folders.
	// If it matches nothing, discovery code will fall back to *.json.
	last := parentType
	if idx := strings.LastIndex(parentType, "/"); idx >= 0 {
		last = parentType[idx+1:]
	}
	if last == "" {
		return []string{"*.json"}
	}
	// Common ARM spec files are PascalCase, e.g. ManagedEnvironments*.json.
	pascal := strings.ToUpper(last[:1]) + last[1:]
	return []string{pascal + "*.json", "*.json"}
}
