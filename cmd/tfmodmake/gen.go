package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
	"github.com/matt-FFFFFF/tfmodmake/schema"
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
			&cli.StringFlag{
				Name:  "resource",
				Usage: "Resource type to generate (e.g., Microsoft.ContainerService/managedClusters)",
			},
			&cli.StringFlag{
				Name:  "local-name",
				Usage: "Name of the local variable to generate (default: resource_body)",
			},
			&cli.StringFlag{
				Name:  "api-version",
				Usage: "Specific API version to use",
			},
			&cli.BoolFlag{
				Name:  "include-preview",
				Usage: "Include latest preview API version",
			},
		},
		Action: runGen,
		Commands: []*cli.Command{
			{
				Name:  "submodule",
				Usage: "Generate a child/submodule and wire it into parent",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "api-version",
						Usage: "Specific API version to use",
					},
					&cli.BoolFlag{
						Name:  "include-preview",
						Usage: "Include latest preview API version",
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
					&cli.StringFlag{
						Name:  "api-version",
						Usage: "Specific API version to use",
					},
					&cli.BoolFlag{
						Name:  "include-preview",
						Usage: "Include latest preview API version",
					},
					&cli.StringFlag{
						Name:     "resource",
						Usage:    "Parent resource type",
						Required: true,
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
	resourceType := cmd.String("resource")
	localName := cmd.String("local-name")
	apiVersion := cmd.String("api-version")
	includePreview := cmd.Bool("include-preview")

	if resourceType == "" {
		return cli.ShowSubcommandHelp(cmd)
	}

	return generateBaseModule(ctx, resourceType, apiVersion, includePreview, localName)
}

func runAddChild(ctx context.Context, cmd *cli.Command) error {
	apiVersion := cmd.String("api-version")
	includePreview := cmd.Bool("include-preview")
	child := cmd.String("child")
	moduleDir := cmd.String("module-dir")
	moduleName := cmd.String("module-name")
	dryRun := cmd.Bool("dry-run")

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
		return nil
	}

	if err := generateChildModule(ctx, child, apiVersion, includePreview, modulePath); err != nil {
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
	resourceType := cmd.String("resource")
	localName := cmd.String("local-name")
	apiVersion := cmd.String("api-version")
	includePreview := cmd.Bool("include-preview")
	moduleDir := cmd.String("module-dir")
	dryRun := cmd.Bool("dry-run")

	if dryRun {
		fmt.Println("DRY RUN: Would execute the following steps:")
		fmt.Printf("1. Generate base module for resource: %s\n", resourceType)
		fmt.Printf("2. Discover children under parent: %s\n", resourceType)
		fmt.Printf("3. Generate submodule for each discovered child in: %s/\n", moduleDir)
		fmt.Printf("4. Generate main.interfaces.tf\n")
		return nil
	}

	if err := orchestrateAVMGeneration(ctx, resourceType, apiVersion, includePreview, localName, moduleDir); err != nil {
		return fmt.Errorf("failed to generate AVM module: %w", err)
	}

	fmt.Println("Successfully generated AVM module with child submodules and interfaces")
	return nil
}

// generateChildModule generates a child module scaffold at the specified path.
func generateChildModule(ctx context.Context, childType, apiVersion string, includePreview bool, modulePath string) error {
	if err := os.MkdirAll(modulePath, 0o755); err != nil {
		return fmt.Errorf("failed to create module directory: %w", err)
	}

	var loadOpts []terraform.LoadOption
	if apiVersion != "" {
		loadOpts = append(loadOpts, terraform.WithAPIVersionLoad(apiVersion))
	}
	loadOpts = append(loadOpts, terraform.WithIncludePreview(includePreview))

	result, err := terraform.LoadResource(ctx, childType, loadOpts...)
	if err != nil {
		return fmt.Errorf("failed to load child resource: %w", err)
	}

	moduleName := deriveModuleName(childType)
	localName := "resource_body"
	if err := terraform.Generate(childType,
		result,
		terraform.WithLocalName(localName),
		terraform.WithModuleNamePrefix(moduleName),
		terraform.WithOutputDir(modulePath),
	); err != nil {
		return fmt.Errorf("failed to generate terraform files: %w", err)
	}

	return nil
}

// orchestrateAVMGeneration performs the full AVM generation workflow
func orchestrateAVMGeneration(ctx context.Context, resourceType, apiVersion string, includePreview bool, localName, moduleDir string) error {
	// Step 1: Generate base module
	fmt.Println("Step 1/4: Generating base module...")
	if err := generateBaseModule(ctx, resourceType, apiVersion, includePreview, localName); err != nil {
		return fmt.Errorf("failed to generate base module: %w", err)
	}

	// Step 2: Discover children from bicep-types index
	fmt.Println("Step 2/4: Discovering child resources...")
	indexData, err := bicepdata.FetchIndex(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch bicep-types index: %w", err)
	}
	idx, err := bicepdata.ParseIndex(indexData)
	if err != nil {
		return fmt.Errorf("failed to parse bicep-types index: %w", err)
	}
	children := schema.DiscoverChildren(idx, resourceType, 1)

	fmt.Printf("Found %d child resource type(s)\n", len(children))

	// Step 3: Generate submodule for each child
	if len(children) > 0 {
		fmt.Println("Step 3/4: Generating child submodules...")
		for i, child := range children {
			if isInterfaceManagedChild(child.ResourceType) {
				fmt.Printf("  [%d/%d] Skipping interface-managed child %s\n", i+1, len(children), child.ResourceType)
				continue
			}

			fmt.Printf("  [%d/%d] Generating submodule for %s...\n", i+1, len(children), child.ResourceType)

			moduleName := deriveModuleName(child.ResourceType)
			modulePath := filepath.Join(moduleDir, moduleName)

			if err := generateChildModule(ctx, child.ResourceType, apiVersion, includePreview, modulePath); err != nil {
				return fmt.Errorf("failed to generate child module for %s: %w", child.ResourceType, err)
			}

			if err := submodule.Generate(modulePath); err != nil {
				return fmt.Errorf("failed to wire child module for %s: %w", child.ResourceType, err)
			}
		}
	} else {
		fmt.Println("Step 3/4: No child resources found, skipping submodule generation")
	}

	// Step 4: Generate AVM interfaces
	fmt.Println("Step 4/4: Generating AVM interfaces...")
	var rs *schema.ResourceSchema
	loaded, loadErr := bicepdata.LoadResourceFromIndex(ctx, idx, resourceType, apiVersion, includePreview, nil)
	if loadErr == nil {
		rs, _ = schema.ConvertResource(loaded)
	}
	if err := terraform.GenerateInterfacesFile(resourceType, rs, "."); err != nil {
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
func generateBaseModule(ctx context.Context, resourceType, apiVersion string, includePreview bool, localName string) error {
	var loadOpts []terraform.LoadOption
	if apiVersion != "" {
		loadOpts = append(loadOpts, terraform.WithAPIVersionLoad(apiVersion))
	}
	loadOpts = append(loadOpts, terraform.WithIncludePreview(includePreview))

	result, err := terraform.LoadResource(ctx, resourceType, loadOpts...)
	if err != nil {
		return fmt.Errorf("failed to load resource: %w", err)
	}

	finalLocalName := "resource_body"
	if localName != "" {
		finalLocalName = localName
	}

	return terraform.Generate(resourceType,
		result,
		terraform.WithLocalName(finalLocalName),
	)
}
