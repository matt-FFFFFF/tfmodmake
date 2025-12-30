package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/matt-FFFFFF/tfmodmake/naming"
	"github.com/matt-FFFFFF/tfmodmake/openapi"
	specpkg "github.com/matt-FFFFFF/tfmodmake/specs"
	"github.com/matt-FFFFFF/tfmodmake/submodule"
	"github.com/matt-FFFFFF/tfmodmake/terraform"
	"github.com/urfave/cli/v3"
)

func GenCommand() *cli.Command {
	return &cli.Command{
		Name:    "gen",
		Aliases: []string{"g"},
		Usage:   "Generate base module",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "spec",
				Usage: "Path or URL to the OpenAPI specification",
			},
			&cli.StringFlag{
				Name:  "resource",
				Usage: "Resource type to generate (e.g., Microsoft.ContainerService/managedClusters)",
			},
			&cli.StringFlag{
				Name:  "root",
				Usage: "Path to the root object (e.g., properties or properties.foo)",
				Value: "properties",
			},
			&cli.StringFlag{
				Name:  "local-name",
				Usage: "Name of the local variable to generate (default: resource_body or derived from root)",
			},
		},
		Action: runGen,
		Commands: []*cli.Command{
			{
				Name:  "submodule",
				Usage: "Generate a child/submodule and wire it into parent",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "spec",
						Usage: "Path or URL to OpenAPI spec",
					},
					&cli.StringFlag{
						Name:  "spec-root",
						Usage: "GitHub tree URL under Azure/azure-rest-api-specs",
					},
					&cli.BoolFlag{
						Name:  "include-preview",
						Usage: "Include latest preview API version",
					},
					&cli.StringFlag{
						Name:  "include",
						Value: "*.json",
						Usage: "Glob filter used when discovering spec files",
					},
					&cli.StringFlag{
						Name:     "parent",
						Usage:    "Parent resource type",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "child",
						Usage:    "Child resource type",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "module-dir",
						Value: "modules",
						Usage: "Directory where child modules live",
					},
					&cli.StringFlag{
						Name:  "module-name",
						Usage: "Override derived module folder name",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Print planned actions without writing files",
					},
				},
				Action: runAddChild,
			},
			{
				Name:  "avm",
				Usage: "Generate base module + child submodules + AVM interfaces",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "spec",
						Usage: "Path or URL to OpenAPI spec",
					},
					&cli.StringFlag{
						Name:  "spec-root",
						Usage: "GitHub tree URL under Azure/azure-rest-api-specs",
					},
					&cli.BoolFlag{
						Name:  "include-preview",
						Usage: "Include latest preview API version",
					},
					&cli.BoolFlag{
						Name:  "print-resolved-specs",
						Usage: "Print resolved spec list to stderr",
					},
					&cli.StringFlag{
						Name:     "resource",
						Usage:    "Parent resource type",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "root",
						Usage: "Path to the root object",
					},
					&cli.StringFlag{
						Name:  "local-name",
						Usage: "Name of the local variable to generate",
					},
					&cli.StringFlag{
						Name:  "module-dir",
						Value: "modules",
						Usage: "Directory where child modules live",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Print planned actions without writing files",
					},
				},
				Action: runGenAVM,
			},
		},
	}
}

func runGen(ctx context.Context, cmd *cli.Command) error {
	specs := cmd.StringSlice("spec")
	resourceType := cmd.String("resource")
	rootPath := cmd.String("root")
	localName := cmd.String("local-name")

	if len(specs) == 0 || resourceType == "" {
		return cli.ShowSubcommandHelp(cmd)
	}

	return generateBaseModule(ctx, specs, resourceType, rootPath, localName)
}

func runAddChild(ctx context.Context, cmd *cli.Command) error {
	specs := cmd.StringSlice("spec")
	specRoot := cmd.String("spec-root")
	includePreview := cmd.Bool("include-preview")
	includeGlob := cmd.String("include")
	parent := cmd.String("parent")
	child := cmd.String("child")
	moduleDir := cmd.String("module-dir")
	moduleName := cmd.String("module-name")
	dryRun := cmd.Bool("dry-run")

	if len(specs) == 0 && specRoot == "" {
		return fmt.Errorf("at least one -spec or -spec-root is required")
	}

	githubToken := specpkg.GithubTokenFromEnv()

	includeGlobs := []string{includeGlob}
	if includeGlob == "*.json" && parent != "" {
		includeGlobs = defaultDiscoveryGlobsForParent(parent)
	}

	resolver := specpkg.DefaultSpecResolver{}
	resolveReq := specpkg.ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: specRoot,
		DiscoverFromSeed:  false,
		IncludeGlobs:      includeGlobs,
		IncludePreview:    includePreview,
		GitHubToken:       githubToken,
	}
	resolved, err := resolver.Resolve(ctx, resolveReq)
	if err != nil {
		return fmt.Errorf("failed to resolve specs: %w", err)
	}

	specSources := make([]string, 0, len(resolved.Specs))
	for _, spec := range resolved.Specs {
		if spec.Source == "" {
			continue
		}
		specSources = append(specSources, spec.Source)
	}

	if len(specSources) == 0 {
		return fmt.Errorf("no specs resolved. Please provide -spec or -spec-root")
	}

	finalModuleName := moduleName
	if finalModuleName == "" {
		finalModuleName = deriveModuleName(child)
	}

	modulePath := filepath.Join(moduleDir, finalModuleName)

	if dryRun {
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
		return nil
	}

	if err := generateChildModule(ctx, specSources, child, modulePath); err != nil {
		return fmt.Errorf("failed to generate child module: %w", err)
	}

	if err := submodule.Generate(modulePath); err != nil {
		return fmt.Errorf("failed to wire child module: %w", err)
	}

	fmt.Printf("Successfully created child module at: %s\n", modulePath)
	fmt.Println("Successfully generated submodule wrapper files")
	return nil
}

func runGenAVM(ctx context.Context, cmd *cli.Command) error {
	specs := cmd.StringSlice("spec")
	specRoot := cmd.String("spec-root")
	includePreview := cmd.Bool("include-preview")
	printResolvedSpecs := cmd.Bool("print-resolved-specs")
	resourceType := cmd.String("resource")
	rootPath := cmd.String("root")
	localName := cmd.String("local-name")
	moduleDir := cmd.String("module-dir")
	dryRun := cmd.Bool("dry-run")

	if len(specs) == 0 && specRoot == "" {
		return fmt.Errorf("at least one -spec or -spec-root is required")
	}

	githubToken := specpkg.GithubTokenFromEnv()
	includeGlobs := defaultDiscoveryGlobsForParent(resourceType)

	resolver := specpkg.DefaultSpecResolver{}
	resolveReq := specpkg.ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: specRoot,
		DiscoverFromSeed:  false,
		IncludeGlobs:      includeGlobs,
		IncludePreview:    includePreview,
		GitHubToken:       githubToken,
	}
	resolved, err := resolver.Resolve(ctx, resolveReq)
	if err != nil {
		return fmt.Errorf("failed to resolve specs: %w", err)
	}

	if printResolvedSpecs {
		specpkg.WriteResolvedSpecs(os.Stderr, resolved.Specs)
	}

	specSources := make([]string, 0, len(resolved.Specs))
	for _, spec := range resolved.Specs {
		if spec.Source == "" {
			continue
		}
		specSources = append(specSources, spec.Source)
	}

	if len(specSources) == 0 {
		return fmt.Errorf("no specs resolved. Please provide -spec or -spec-root")
	}

	if dryRun {
		fmt.Println("DRY RUN: Would execute the following steps:")
		fmt.Printf("1. Generate base module for resource: %s\n", resourceType)
		fmt.Printf("2. Discover children under parent: %s\n", resourceType)
		fmt.Printf("3. Generate submodule for each discovered child in: %s/\n", moduleDir)
		fmt.Printf("4. Generate main.interfaces.tf\n")
		fmt.Printf("Using %d resolved spec(s)\n", len(specSources))
		return nil
	}

	if err := orchestrateAVMGeneration(ctx, specSources, resourceType, rootPath, localName, moduleDir); err != nil {
		return fmt.Errorf("failed to generate AVM module: %w", err)
	}

	fmt.Println("Successfully generated AVM module with child submodules and interfaces")
	return nil
}

// generateChildModule generates a child module scaffold at the specified path.
func generateChildModule(ctx context.Context, specs []string, childType, modulePath string) error {
	// Create module directory if it doesn't exist
	if err := os.MkdirAll(modulePath, 0o755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	// Load specs and find the child resource
	// Load the child resource from specs
	result, err := LoadResourceFromSpecs(ctx, specs, childType)
	if err != nil {
		return fmt.Errorf("failed to load child resource: %w", err)
	}

	schema := result.Schema
	doc := result.Doc
	apiVersion := result.APIVersion
	nameSchema := result.NameSchema
	supportsTags := result.SupportsTags
	supportsLocation := result.SupportsLocation

	// Derive module name for variable renaming context
	moduleName := deriveModuleName(childType)

	// Ensure module directory exists
	if err := os.MkdirAll(modulePath, 0o755); err != nil {
		return fmt.Errorf("failed to create module directory %s: %w", modulePath, err)
	}

	// Generate Terraform files in the module directory
	localName := "resource_body"
	if err := terraform.GenerateWithContext(schema, childType, localName, apiVersion, supportsTags, supportsLocation, nameSchema, doc, moduleName, modulePath); err != nil {
		return fmt.Errorf("failed to generate terraform files: %w", err)
	}

	return nil
}

// orchestrateAVMGeneration performs the full AVM generation workflow
func orchestrateAVMGeneration(ctx context.Context, specSources []string, resourceType, rootPath, localName, moduleDir string) error {
	// Step 1: Generate base module
	fmt.Println("Step 1/4: Generating base module...")
	if err := generateBaseModule(ctx, specSources, resourceType, rootPath, localName); err != nil {
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
			// Some child resource types are managed via AVM interfaces on the parent module.
			// For example, private endpoints are configured through the interfaces module and
			// should not be generated as a standalone child submodule.
			if isInterfaceManagedChild(child.ResourceType) {
				fmt.Printf("  [%d/%d] Skipping interface-managed child %s\n", i+1, len(result.Deployable), child.ResourceType)
				continue
			}

			fmt.Printf("  [%d/%d] Generating submodule for %s...\n", i+1, len(result.Deployable), child.ResourceType)

			// Derive module name from child type
			moduleName := deriveModuleName(child.ResourceType)
			modulePath := filepath.Join(moduleDir, moduleName)

			// Generate child module
			if err := generateChildModule(ctx, specSources, child.ResourceType, modulePath); err != nil {
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
	// Load the spec for capability detection (reuse the logic from generateBaseModule)
	var doc *openapi3.T
	for _, specPath := range specSources {
		loadedDoc, err := openapi.LoadSpec(specPath)
		if err != nil {
			continue
		}
		// Verify this spec contains the resource
		if _, err := openapi.FindResource(loadedDoc, resourceType); err == nil {
			doc = loadedDoc
			break
		}
	}
	if err := terraform.GenerateInterfacesFile(resourceType, doc, "."); err != nil {
		return fmt.Errorf("failed to generate AVM interfaces: %w", err)
	}

	return nil
}

func isInterfaceManagedChild(childResourceType string) bool {
	// Today, the only known interface-managed child we want to suppress is Private Endpoint Connections.
	// The interfaces module handles private endpoints through the  input.
	last := childResourceType
	if idx := strings.LastIndex(childResourceType, "/"); idx >= 0 {
		last = childResourceType[idx+1:]
	}
	return strings.EqualFold(last, "privateEndpointConnections")
}

// generateBaseModule generates the base module files in the current directory
func generateBaseModule(ctx context.Context, specSources []string, resourceType, rootPath, localName string) error {
	// Load the resource from specs
	result, err := LoadResourceFromSpecs(ctx, specSources, resourceType)
	if err != nil {
		return fmt.Errorf("failed to load resource: %w", err)
	}

	schema := result.Schema
	doc := result.Doc
	apiVersion := result.APIVersion
	nameSchema := result.NameSchema
	supportsTags := result.SupportsTags
	supportsLocation := result.SupportsLocation

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
	return terraform.Generate(schema, resourceType, finalLocalName, apiVersion, supportsTags, supportsLocation, nameSchema, doc)
}
