package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	specpkg "github.com/matt-FFFFFF/tfmodmake/specs"
	"github.com/matt-FFFFFF/tfmodmake/terraform"
	"github.com/urfave/cli/v3"
)

// UpdateCommand returns the CLI command for updating an existing module to a new API version.
func UpdateCommand() *cli.Command {
	return &cli.Command{
		Name:    "update",
		Aliases: []string{"u"},
		Usage:   "Update an existing module to a new API version",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "spec",
				Usage: "Path or URL to the new OpenAPI spec",
			},
			&cli.StringFlag{
				Name:  "spec-root",
				Usage: "GitHub tree URL for spec discovery",
			},
			&cli.StringFlag{
				Name:  "resource",
				Usage: "Resource type (inferred from main.tf if omitted)",
			},
			&cli.BoolFlag{
				Name:  "include-preview",
				Usage: "Include preview API versions during spec resolution",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Print planned changes without modifying files",
			},
		},
		Action: runUpdate,
	}
}

func runUpdate(ctx context.Context, cmd *cli.Command) error {
	specs := cmd.StringSlice("spec")
	specRoot := cmd.String("spec-root")
	includePreview := cmd.Bool("include-preview")
	resourceType := cmd.String("resource")
	dryRun := cmd.Bool("dry-run")

	if len(specs) == 0 && specRoot == "" {
		return fmt.Errorf("at least one --spec or --spec-root is required")
	}

	// If resource type not provided, infer from main.tf
	if resourceType == "" {
		inferred, err := inferResourceTypeFromMainTf()
		if err != nil {
			return fmt.Errorf("could not infer resource type from main.tf (use --resource to specify): %w", err)
		}
		// Strip the @apiVersion suffix if present
		if idx := strings.LastIndex(inferred, "@"); idx > 0 {
			resourceType = inferred[:idx]
		} else {
			resourceType = inferred
		}
	}

	githubToken := specpkg.GithubTokenFromEnv()
	includeGlobs := defaultDiscoveryGlobsForParent(resourceType)

	// Extract the old API version from the existing module's main.tf so we can
	// resolve the old spec for a proper 3-way comparison.
	oldVersion, err := extractOldVersionFromMainTf()
	if err != nil {
		return fmt.Errorf("could not extract old API version from main.tf: %w", err)
	}

	// Resolve old specs using PinVersion to get the baseline spec.
	var oldSpecSources []string
	if specRoot != "" {
		resolver := specpkg.DefaultSpecResolver{}
		oldResolveReq := specpkg.ResolveRequest{
			GitHubServiceRoot: specRoot,
			IncludeGlobs:      includeGlobs,
			PinVersion:        oldVersion,
			GitHubToken:       githubToken,
		}
		oldResolved, resolveErr := resolver.Resolve(ctx, oldResolveReq)
		if resolveErr != nil {
			return fmt.Errorf("failed to resolve old specs for version %s: %w", oldVersion, resolveErr)
		}
		for _, spec := range oldResolved.Specs {
			if spec.Source != "" {
				oldSpecSources = append(oldSpecSources, spec.Source)
			}
		}
	}
	// If we couldn't resolve old specs via spec-root, fall back to seed specs
	// (the caller may have provided old specs directly).
	if len(oldSpecSources) == 0 {
		oldSpecSources = specs
	}

	// Resolve new specs (latest version).
	resolver := specpkg.DefaultSpecResolver{}
	newResolveReq := specpkg.ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: specRoot,
		DiscoverFromSeed:  false,
		IncludeGlobs:      includeGlobs,
		IncludePreview:    includePreview,
		GitHubToken:       githubToken,
	}
	newResolved, err := resolver.Resolve(ctx, newResolveReq)
	if err != nil {
		return fmt.Errorf("failed to resolve new specs: %w", err)
	}

	newSpecSources := make([]string, 0, len(newResolved.Specs))
	for _, spec := range newResolved.Specs {
		if spec.Source != "" {
			newSpecSources = append(newSpecSources, spec.Source)
		}
	}
	if len(newSpecSources) == 0 {
		return fmt.Errorf("no new specs resolved. Please provide --spec or --spec-root")
	}

	// Run update with separate old and new specs for 3-way comparison.
	result, err := terraform.Update(ctx, terraform.UpdateOptions{
		ModuleDir:    ".",
		OldSpecs:     oldSpecSources,
		NewSpecs:     newSpecSources,
		ResourceType: resourceType,
		DryRun:       dryRun,
	})
	if err != nil {
		return err
	}

	// Print summary
	printUpdateSummary(result, dryRun)
	return nil
}

// extractOldVersionFromMainTf reads main.tf and extracts the old API version.
func extractOldVersionFromMainTf() (string, error) {
	mainFile, err := terraform.ParseModuleFile(".", "main.tf")
	if err != nil {
		return "", fmt.Errorf("reading main.tf: %w", err)
	}
	_, version, err := terraform.ExtractResourceTypeAndVersion(mainFile)
	if err != nil {
		return "", err
	}
	return version, nil
}

func printUpdateSummary(result *terraform.UpdateResult, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "DRY RUN: "
	}

	fmt.Printf("%sAPI version: %s -> %s\n\n", prefix, result.OldVersion, result.NewVersion)

	printItemSummary(prefix+"Variables", result.Variables)
	printItemSummary(prefix+"Locals", result.Locals)

	if result.MainUpdated {
		fmt.Printf("%sMain: type attribute updated\n", prefix)
	}
	if result.OutputsRegenerated {
		fmt.Printf("%sOutputs: regenerated from new spec\n", prefix)
	}
}

func printItemSummary(header string, summary terraform.UpdateSummary) {
	hasChanges := len(summary.AutoUpdated) > 0 || len(summary.Added) > 0 ||
		len(summary.Removed) > 0 || len(summary.NeedsReview) > 0

	if !hasChanges {
		return
	}

	fmt.Printf("%s:\n", header)
	printSortedItems("  auto-updated", summary.AutoUpdated)
	printSortedItems("  added", summary.Added)
	printSortedItems("  removed", summary.Removed)
	printSortedItems("  needs review", summary.NeedsReview)
	fmt.Println()
}

func printSortedItems(prefix string, items []string) {
	if len(items) == 0 {
		return
	}
	sorted := make([]string, len(items))
	copy(sorted, items)
	sort.Strings(sorted)
	for _, item := range sorted {
		fmt.Printf("%s: %s\n", prefix, item)
	}
}
