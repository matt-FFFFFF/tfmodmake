package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

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
