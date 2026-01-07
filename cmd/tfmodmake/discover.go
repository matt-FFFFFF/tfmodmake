package main

import (
	"context"
	"fmt"
	"os"

	"github.com/matt-FFFFFF/tfmodmake/openapi"
	specpkg "github.com/matt-FFFFFF/tfmodmake/specs"
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
					&cli.StringSliceFlag{
						Name:  "spec",
						Usage: "Path or URL to OpenAPI spec",
					},
					&cli.StringFlag{
						Name:  "spec-root",
						Usage: "GitHub tree URL under Azure/azure-rest-api-specs",
					},
					&cli.BoolFlag{
						Name:  "discover",
						Usage: "Discover additional spec files",
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
					&cli.BoolFlag{
						Name:  "json",
						Usage: "Output results as JSON",
					},
					&cli.BoolFlag{
						Name:  "print-resolved-specs",
						Usage: "Print the resolved spec list to stderr",
					},
				},
				Action: runDiscoverChildren,
			},
		},
	}
}

func runDiscoverChildren(ctx context.Context, cmd *cli.Command) error {
	specs := cmd.StringSlice("spec")
	specRoot := cmd.String("spec-root")
	discoverFromSpec := cmd.Bool("discover")
	includePreview := cmd.Bool("include-preview")
	includeGlob := cmd.String("include")
	parent := cmd.String("parent")
	jsonOutput := cmd.Bool("json")
	printResolvedSpecs := cmd.Bool("print-resolved-specs")

	githubToken := specpkg.GithubTokenFromEnv()

	includeGlobs := []string{includeGlob}
	if includeGlob == "*.json" && parent != "" {
		includeGlobs = defaultDiscoveryGlobsForParent(parent)
	}

	resolver := specpkg.DefaultSpecResolver{}
	resolveReq := specpkg.ResolveRequest{
		Seeds:             specs,
		GitHubServiceRoot: specRoot,
		DiscoverFromSeed:  discoverFromSpec,
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

	opts := openapi.DiscoverChildrenOptions{
		Specs:  specSources,
		Parent: parent,
		Depth:  1,
	}

	result, err := openapi.DiscoverChildren(opts)
	if err != nil {
		return fmt.Errorf("failed to discover children: %w", err)
	}

	if jsonOutput {
		jsonStr, err := openapi.FormatChildrenAsJSON(result)
		if err != nil {
			return fmt.Errorf("failed to format as JSON: %w", err)
		}
		fmt.Println(jsonStr)
	} else {
		text := openapi.FormatChildrenAsText(result)
		fmt.Print(text)
	}
	return nil
}
