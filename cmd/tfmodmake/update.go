package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
			&cli.StringFlag{
				Name:  "api-version",
				Usage: "New API version to update to (latest stable if omitted)",
			},
			&cli.StringFlag{
				Name:  "resource",
				Usage: "Resource type (inferred from main.tf if omitted)",
			},
			&cli.BoolFlag{
				Name:  "include-preview",
				Usage: "Include preview API versions when resolving latest",
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
	apiVersion := cmd.String("api-version")
	includePreview := cmd.Bool("include-preview")
	resourceType := cmd.String("resource")
	dryRun := cmd.Bool("dry-run")

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

	// Run update with the new API version
	result, err := terraform.Update(ctx, terraform.UpdateOptions{
		ModuleDir:      ".",
		NewAPIVersion:  apiVersion,
		ResourceType:   resourceType,
		IncludePreview: includePreview,
		DryRun:         dryRun,
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
