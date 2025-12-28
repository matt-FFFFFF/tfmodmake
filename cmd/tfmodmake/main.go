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

func printUsage() {
	fmt.Fprintf(os.Stderr, `tfmodmake - Generate Terraform modules from OpenAPI specifications

Usage:
  tfmodmake [command] [flags]

Commands:
  (default)              Generate base module (same as 'gen')
  gen                    Generate base module
  gen submodule          Generate a child/submodule and wire it into parent
  gen avm                Generate base module + child submodules + AVM interfaces
  add submodule <path>   Generate wrapper for an existing submodule
  add avm-interfaces [path]  Generate main.interfaces.tf (infers resource from main.tf)
  discover children      List deployable child resource types

Flags for base generation (default / gen):
  -spec string
        Path or URL to the OpenAPI specification (required)
  -resource string
        Resource type to generate (e.g., Microsoft.ContainerService/managedClusters) (required)
  -root string
        Path to the root object (e.g., properties or properties.foo) (optional)
  -local-name string
        Name of the local variable to generate (default: resource_body or derived from root) (optional)

Examples:
  # Generate base module for AKS
  tfmodmake -spec <url> -resource Microsoft.ContainerService/managedClusters

  # Generate base module (explicit form)
  tfmodmake gen -spec <url> -resource Microsoft.ContainerService/managedClusters

  # Add AVM interfaces scaffolding (infers resource type from main.tf in current directory)
  tfmodmake add avm-interfaces

  # Add AVM interfaces scaffolding to a specific directory
  tfmodmake add avm-interfaces path/to/module

  # Generate wrapper for existing submodule
  tfmodmake add submodule modules/secrets

  # Generate child module
  tfmodmake gen submodule -parent Microsoft.App/managedEnvironments \
    -child Microsoft.App/managedEnvironments/storages \
    -spec-root <github_tree_url>

  # Discover child resources
  tfmodmake discover children -parent Microsoft.App/managedEnvironments \
    -spec-root <github_tree_url>

For more information, visit: https://github.com/matt-FFFFFF/tfmodmake
`)
}

func main() {
	// Show help for -h or --help at top level
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		printUsage()
		os.Exit(0)
	}

	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "gen":
			handleGenCommand()
			return
		case "add":
			handleAddCommand()
			return
		case "discover":
			handleDiscoverCommand()
			return
		}
	}

	// Default: base module generation (same as 'gen' with no subcommand)
	handleDefaultGeneration()
}

func handleGenCommand() {
	if len(os.Args) > 2 {
		switch os.Args[2] {
		case "submodule":
			// tfmodmake gen submodule - handled by addchild logic
			handleAddChildCommand()
			return
		case "avm":
			// tfmodmake gen avm - orchestrator for AVM module generation
			handleGenAVMCommand()
			return
		}
	}

	// tfmodmake gen - same as default generation
	// Remove "gen" from args so flag parsing works correctly
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	handleDefaultGeneration()
}

func handleAddCommand() {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: %s add <subcommand>\nAvailable subcommands: submodule, avm-interfaces", os.Args[0])
	}

	switch os.Args[2] {
	case "submodule":
		handleAddSubCommand()
	case "avm-interfaces":
		handleAddAVMInterfacesCommand()
	default:
		log.Fatalf("Unknown add subcommand: %s\nAvailable subcommands: submodule, avm-interfaces", os.Args[2])
	}
}

func handleDiscoverCommand() {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: %s discover <subcommand>\nAvailable subcommands: children", os.Args[0])
	}

	switch os.Args[2] {
	case "children":
		handleChildrenCommand()
	default:
		log.Fatalf("Unknown discover subcommand: %s\nAvailable subcommands: children", os.Args[2])
	}
}

func handleAddSubCommand() {
	// Parse args differently based on how we got here
	var args []string
	if len(os.Args) > 1 && os.Args[1] == "addsub" {
		// Legacy: tfmodmake addsub <path>
		addSub := flag.NewFlagSet("addsub", flag.ExitOnError)
		if err := addSub.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse addsub arguments: %v", err)
		}
		args = addSub.Args()
	} else {
		// New: tfmodmake add submodule <path>
		addSub := flag.NewFlagSet("add submodule", flag.ExitOnError)
		if err := addSub.Parse(os.Args[3:]); err != nil {
			log.Fatalf("Failed to parse add submodule arguments: %v", err)
		}
		args = addSub.Args()
	}

	if len(args) != 1 {
		log.Fatalf("Usage: %s add submodule <path>", os.Args[0])
	}
	if err := submodule.Generate(args[0]); err != nil {
		log.Fatalf("Failed to add submodule: %v", err)
	}
	fmt.Println("Successfully generated submodule wrapper files")
}

func handleAddAVMInterfacesCommand() {
	// tfmodmake add avm-interfaces [path]
	addAVMCmd := flag.NewFlagSet("add avm-interfaces", flag.ExitOnError)

	if err := addAVMCmd.Parse(os.Args[3:]); err != nil {
		log.Fatalf("Failed to parse add avm-interfaces arguments: %v", err)
	}

	// Get optional path argument (defaults to current directory)
	targetDir := "."
	args := addAVMCmd.Args()
	if len(args) > 0 {
		targetDir = args[0]
	}

	// Save current directory to restore later
	originalDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Change to target directory
	if err := os.Chdir(targetDir); err != nil {
		log.Fatalf("Failed to change to directory %s: %v", targetDir, err)
	}
	defer os.Chdir(originalDir)

	// Infer resource type from main.tf (required)
	finalResourceType, err := inferResourceTypeFromMainTf()
	if err != nil {
		log.Fatalf("Failed to infer resource type from main.tf: %v\nEnsure main.tf exists in %s", err, targetDir)
	}

	if err := terraform.GenerateInterfacesFile(finalResourceType); err != nil {
		log.Fatalf("Failed to generate AVM interfaces: %v", err)
	}

	fmt.Println("Successfully generated main.interfaces.tf")
}

func handleDefaultGeneration() {
	genCmd := flag.NewFlagSet("gen", flag.ExitOnError)
	genCmd.Usage = func() {
		printUsage()
	}
	
	specPath := genCmd.String("spec", "", "Path or URL to the OpenAPI specification")
	resourceType := genCmd.String("resource", "", "Resource type to generate Terraform configuration for (e.g. Microsoft.ContainerService/managedClusters)")
	rootPath := genCmd.String("root", "", "Path to the root object (e.g. properties or properties.foo)")
	localName := genCmd.String("local-name", "", "Name of the local variable to generate (default: resource_body or derived from root)")
	genCmd.Parse(os.Args[1:])

	if *specPath == "" || *resourceType == "" {
		printUsage()
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
	// Determine the command name and adjust args based on how we got here
	var cmdName string
	var argsOffset int
	
	if len(os.Args) > 1 && os.Args[1] == "children" {
		// Legacy: tfmodmake children ...
		cmdName = "children"
		argsOffset = 2
	} else if len(os.Args) > 2 && os.Args[1] == "discover" && os.Args[2] == "children" {
		// New: tfmodmake discover children ...
		cmdName = "discover children"
		argsOffset = 3
	} else {
		log.Fatalf("Unexpected command structure for children")
	}

	childrenCmd := flag.NewFlagSet(cmdName, flag.ExitOnError)

	var specs stringSliceFlag
	childrenCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
	specRoot := childrenCmd.String("spec-root", "", "GitHub tree URL under Azure/azure-rest-api-specs (https://github.com/Azure/azure-rest-api-specs/tree/<ref>/<dir>)")
	discoverFromSpec := childrenCmd.Bool("discover", false, "Discover additional spec files from the directory of the provided raw.githubusercontent.com -spec URL")
	includePreview := childrenCmd.Bool("include-preview", false, "When discovering specs from a GitHub service root, also include the latest preview API version folder")
	includeGlob := childrenCmd.String("include", "*.json", "Glob filter used when discovering spec files (matched against filename, e.g. ManagedEnvironments*.json)")
	parent := childrenCmd.String("parent", "", "Parent resource type (e.g. Microsoft.App/managedEnvironments)")
	jsonOutput := childrenCmd.Bool("json", false, "Output results as JSON instead of markdown")
	printResolvedSpecs := childrenCmd.Bool("print-resolved-specs", false, "Print the resolved spec list to stderr before analysis")

	if err := childrenCmd.Parse(os.Args[argsOffset:]); err != nil {
		log.Fatalf("Failed to parse children arguments: %v", err)
	}

	githubToken := githubTokenFromEnv()

	includeGlobs := []string{*includeGlob}
	if *includeGlob == "*.json" && *parent != "" {
		includeGlobs = defaultDiscoveryGlobsForParent(*parent)
	}

	resolver := defaultSpecResolver{}
	resolveReq := ResolveRequest{
		Seeds:             specs,
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
	// Determine command name and args offset based on how we got here
	var cmdName string
	var argsOffset int
	
	if len(os.Args) > 1 && os.Args[1] == "addchild" {
		// Legacy: tfmodmake addchild ...
		cmdName = "addchild"
		argsOffset = 2
	} else if len(os.Args) > 2 && os.Args[1] == "gen" && os.Args[2] == "submodule" {
		// New: tfmodmake gen submodule ...
		cmdName = "gen submodule"
		argsOffset = 3
	} else {
		log.Fatalf("Unexpected command structure for addchild/gen submodule")
	}

	addChildCmd := flag.NewFlagSet(cmdName, flag.ExitOnError)

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

	if err := addChildCmd.Parse(os.Args[argsOffset:]); err != nil {
		log.Fatalf("Failed to parse %s arguments: %v", cmdName, err)
	}

	const addChildUsage = "Usage: %s %s -parent <resource_type> -child <resource_type> [-spec <path_or_url>] [-spec-root <url>] [-module-dir <path>] [-module-name <name>] [-dry-run]"

	if *parent == "" {
		log.Fatalf(addChildUsage+"\n-parent is required", os.Args[0], cmdName)
	}

	if *child == "" {
		log.Fatalf(addChildUsage+"\n-child is required", os.Args[0], cmdName)
	}

	if len(specs) == 0 && *specRoot == "" {
		log.Fatalf(addChildUsage+"\nAt least one -spec or -spec-root is required", os.Args[0], cmdName)
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

// inferResourceTypeFromMainTf attempts to read the resource type from an existing main.tf file.
func inferResourceTypeFromMainTf() (string, error) {
	data, err := os.ReadFile("main.tf")
	if err != nil {
		return "", fmt.Errorf("could not read main.tf: %w", err)
	}

	// Look for type = "..." in azapi_resource block
	// This is a simple string search approach
	content := string(data)
	lines := strings.Split(content, "\n")
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "type") && strings.Contains(trimmed, "=") {
			// Extract the value between quotes
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

// handleGenAVMCommand orchestrates end-to-end AVM module generation:
// 1) Generate base module
// 2) Discover children
// 3) Generate submodule for each child
// 4) Generate AVM interfaces
func handleGenAVMCommand() {
	genAVMCmd := flag.NewFlagSet("gen avm", flag.ExitOnError)

	// Spec resolution flags (same as discover children / gen submodule)
	var specs stringSliceFlag
	genAVMCmd.Var(&specs, "spec", "Path or URL to OpenAPI spec (can be specified multiple times)")
	specRoot := genAVMCmd.String("spec-root", "", "GitHub tree URL under Azure/azure-rest-api-specs")
	includePreview := genAVMCmd.Bool("include-preview", false, "Include latest preview API version when discovering specs")
	printResolvedSpecs := genAVMCmd.Bool("print-resolved-specs", false, "Print resolved spec list to stderr")

	// Parent resource selection
	resourceType := genAVMCmd.String("resource", "", "Parent resource type (e.g. Microsoft.App/managedEnvironments)")

	// Base generation options
	rootPath := genAVMCmd.String("root", "", "Path to the root object (e.g. properties or properties.foo)")
	localName := genAVMCmd.String("local-name", "", "Name of the local variable to generate")

	// Submodule options
	moduleDir := genAVMCmd.String("module-dir", "modules", "Directory where child modules live")

	// Dry run
	dryRun := genAVMCmd.Bool("dry-run", false, "Print planned actions without writing files")

	if err := genAVMCmd.Parse(os.Args[3:]); err != nil {
		log.Fatalf("Failed to parse gen avm arguments: %v", err)
	}

	// Validate required arguments
	if *resourceType == "" {
		log.Fatalf("Usage: %s gen avm -resource <resource_type> [-spec <path_or_url>] [-spec-root <url>] [flags]\n-resource is required", os.Args[0])
	}

	if len(specs) == 0 && *specRoot == "" {
		log.Fatalf("Usage: %s gen avm -resource <resource_type> [-spec <path_or_url>] [-spec-root <url>] [flags]\nAt least one -spec or -spec-root is required", os.Args[0])
	}

	// Resolve specs
	githubToken := githubTokenFromEnv()
	includeGlobs := defaultDiscoveryGlobsForParent(*resourceType)

	resolver := defaultSpecResolver{}
	resolveReq := ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: *specRoot,
		DiscoverFromSeed:  false,
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

	if *dryRun {
		fmt.Println("DRY RUN: Would execute the following steps:")
		fmt.Printf("1. Generate base module for resource: %s\n", *resourceType)
		fmt.Printf("2. Discover children under parent: %s\n", *resourceType)
		fmt.Printf("3. Generate submodule for each discovered child in: %s/\n", *moduleDir)
		fmt.Printf("4. Generate main.interfaces.tf\n")
		fmt.Printf("Using %d resolved spec(s)\n", len(specSources))
		return
	}

	// Execute orchestration
	if err := orchestrateAVMGeneration(specSources, *resourceType, *rootPath, *localName, *moduleDir); err != nil {
		log.Fatalf("Failed to generate AVM module: %v", err)
	}

	fmt.Println("Successfully generated AVM module with child submodules and interfaces")
}

// orchestrateAVMGeneration performs the full AVM generation workflow
func orchestrateAVMGeneration(specSources []string, resourceType, rootPath, localName, moduleDir string) error {
	// Step 1: Generate base module
	fmt.Println("Step 1/4: Generating base module...")
	if err := generateBaseModule(specSources, resourceType, rootPath, localName); err != nil {
		return fmt.Errorf("failed to generate base module: %w", err)
	}

	// Step 2: Discover children
	fmt.Println("Step 2/4: Discovering child resources...")
	opts := openapi.DiscoverChildrenOptions{
		Specs:  specSources,
		Parent: resourceType,
		Depth:  1,
	}
	result, err := openapi.DiscoverChildren(opts)
	if err != nil {
		return fmt.Errorf("failed to discover children: %w", err)
	}

	fmt.Printf("Found %d deployable child resource type(s)\n", len(result.Deployable))

	// Step 3: Generate submodule for each child
	if len(result.Deployable) > 0 {
		fmt.Println("Step 3/4: Generating child submodules...")
		for i, child := range result.Deployable {
			fmt.Printf("  [%d/%d] Generating submodule for %s...\n", i+1, len(result.Deployable), child.ResourceType)

			// Derive module name from child type
			moduleName := deriveModuleName(child.ResourceType)
			modulePath := filepath.Join(moduleDir, moduleName)

			// Generate child module
			if err := generateChildModule(specSources, child.ResourceType, modulePath); err != nil {
				return fmt.Errorf("failed to generate child module for %s: %w", child.ResourceType, err)
			}

			// Wire child module into parent
			if err := submodule.Generate(modulePath); err != nil {
				return fmt.Errorf("failed to wire child module for %s: %w", child.ResourceType, err)
			}
		}
	} else {
		fmt.Println("Step 3/4: No child resources found, skipping submodule generation")
	}

	// Step 4: Generate AVM interfaces
	fmt.Println("Step 4/4: Generating AVM interfaces...")
	if err := terraform.GenerateInterfacesFile(resourceType); err != nil {
		return fmt.Errorf("failed to generate AVM interfaces: %w", err)
	}

	return nil
}

// generateBaseModule generates the base module files in the current directory
func generateBaseModule(specSources []string, resourceType, rootPath, localName string) error {
	var schema *openapi3.Schema
	var nameSchema *openapi3.Schema
	var apiVersion string
	var supportsTags bool
	var supportsLocation bool

	// Find the resource in the specs
	var loadErrors []string
	var searchErrors []string

	for _, specPath := range specSources {
		doc, err := openapi.LoadSpec(specPath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue
		}

		// Try to find the resource in this spec
		foundSchema, err := openapi.FindResource(doc, resourceType)
		if err != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("- %s: %v", specPath, err))
			continue
		}

		// Found the resource!
		schema = foundSchema

		// Get API version
		if doc.Info != nil {
			apiVersion = doc.Info.Version
		}

		// Get name schema
		nameSchema, _ = openapi.FindResourceNameSchema(doc, resourceType)

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
		errMsg := fmt.Sprintf("resource type %s not found in any of the provided specs", resourceType)
		if len(loadErrors) > 0 {
			errMsg += fmt.Sprintf("\n\nSpec load errors:\n%s", strings.Join(loadErrors, "\n"))
		}
		if len(searchErrors) > 0 {
			errMsg += fmt.Sprintf("\n\nSpecs checked:\n%s", strings.Join(searchErrors, "\n"))
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Navigate to root path if specified
	if rootPath != "" {
		navigatedSchema, err := openapi.NavigateSchema(schema, rootPath)
		if err != nil {
			return fmt.Errorf("failed to navigate to root path %s: %w", rootPath, err)
		}
		schema = navigatedSchema
	}

	// Determine local name
	finalLocalName := "resource_body"
	if localName != "" {
		finalLocalName = localName
	} else if rootPath != "" {
		finalLocalName = strings.ReplaceAll(rootPath, ".", "_")
		finalLocalName = naming.ToSnakeCase(finalLocalName)
	}

	// Generate Terraform files
	return terraform.Generate(schema, resourceType, finalLocalName, apiVersion, supportsTags, supportsLocation, nameSchema)
}
