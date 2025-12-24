package main

import (
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
		fmt.Println("Successfully generated variables.submodule.tf and main.submodule.tf")
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

	fmt.Println("Successfully generated variables.tf and locals.tf")
}
