package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/urfave/cli/v3"
)

func DiscoverCommand() *cli.Command {
	return &cli.Command{
		Name:  "discover",
		Usage: "Discover resources",
		Commands: []*cli.Command{
			{
				Name:  "children",
				Usage: "List deployable child resource types",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "parent",
						Usage:    "Parent resource type",
						Required: true,
					},
					&cli.BoolFlag{
						Name:  "json",
						Usage: "Output results as JSON",
					},
				},
				Action: runDiscoverChildren,
			},
			{
				Name:  "versions",
				Usage: "List available API versions for a resource type",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "resource",
						Usage:    "Resource type to list versions for",
						Required: true,
					},
				},
				Action: runDiscoverVersions,
			},
		},
	}
}

func runDiscoverChildren(ctx context.Context, cmd *cli.Command) error {
	parent := cmd.String("parent")
	jsonOutput := cmd.Bool("json")

	indexData, err := bicepdata.FetchIndex(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch bicep-types index: %w", err)
	}
	idx, err := bicepdata.ParseIndex(indexData)
	if err != nil {
		return fmt.Errorf("failed to parse bicep-types index: %w", err)
	}

	children := schema.DiscoverChildren(idx, parent, 1)

	if jsonOutput {
		data, err := json.MarshalIndent(children, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format as JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		if len(children) == 0 {
			fmt.Printf("No child resources found for %s\n", parent)
			return nil
		}
		fmt.Printf("Child resources of %s:\n", parent)
		// Sort children by resource type for consistent output
		sort.Slice(children, func(i, j int) bool {
			return strings.ToLower(children[i].ResourceType) < strings.ToLower(children[j].ResourceType)
		})
		for _, child := range children {
			sort.Strings(child.APIVersions)
			fmt.Printf("  %s (API versions: %s)\n", child.ResourceType, strings.Join(child.APIVersions, ", "))
		}
	}
	return nil
}

func runDiscoverVersions(ctx context.Context, cmd *cli.Command) error {
	resourceType := cmd.String("resource")

	indexData, err := bicepdata.FetchIndex(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch bicep-types index: %w", err)
	}
	idx, err := bicepdata.ParseIndex(indexData)
	if err != nil {
		return fmt.Errorf("failed to parse bicep-types index: %w", err)
	}

	versions := bicepdata.ListVersions(idx, resourceType)
	if len(versions) == 0 {
		return fmt.Errorf("no versions found for resource type %s", resourceType)
	}

	sort.Strings(versions)
	fmt.Printf("Available API versions for %s:\n", resourceType)
	for _, v := range versions {
		fmt.Printf("  %s\n", v)
	}
	return nil
}
