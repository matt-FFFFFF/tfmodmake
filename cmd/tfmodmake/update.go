package main

import (
	"context"
	"fmt"
	"sort"

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
		if idx := len(inferred) - 1; idx >= 0 {
			for i := len(inferred) - 1; i >= 0; i-- {
				if inferred[i] == '@' {
					resourceType = inferred[:i]
					break
				}
			}
		}
		if resourceType == "" {
			resourceType = inferred
		}
	}

	githubToken := specpkg.GithubTokenFromEnv()
	includeGlobs := defaultDiscoveryGlobsForParent(resourceType)

	// Resolve specs
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
		return fmt.Errorf("no specs resolved. Please provide --spec or --spec-root")
	}

	// Run update
	result, err := terraform.Update(ctx, terraform.UpdateOptions{
		ModuleDir:    ".",
		NewSpecs:     specSources,
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
