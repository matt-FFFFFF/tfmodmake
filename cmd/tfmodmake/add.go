package main

import (
	"context"
	"fmt"
	"os"

	"github.com/matt-FFFFFF/tfmodmake/bicepdata"
	"github.com/matt-FFFFFF/tfmodmake/schema"
	"github.com/matt-FFFFFF/tfmodmake/submodule"
	"github.com/matt-FFFFFF/tfmodmake/terraform"
	"github.com/urfave/cli/v3"
)

func AddCommand() *cli.Command {
	return &cli.Command{
		Name:  "add",
		Usage: "Add components to the module",
		Commands: []*cli.Command{
			{
				Name:      "submodule",
				Usage:     "Generate wrapper for an existing submodule",
				ArgsUsage: "<path>",
				Action:    runAddSubmodule,
			},
			{
				Name:      "avm-interfaces",
				Usage:     "Generate main.interfaces.tf",
				ArgsUsage: "[path]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "api-version",
						Usage: "Optional: API version for capability detection",
					},
					&cli.BoolFlag{
						Name:  "include-preview",
						Usage: "Include preview API versions",
					},
				},
				Action: runAddAVMInterfaces,
			},
		},
	}
}

func runAddSubmodule(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return cli.ShowSubcommandHelp(cmd)
	}
	path := cmd.Args().First()
	if err := submodule.Generate(path); err != nil {
		return fmt.Errorf("failed to add submodule: %w", err)
	}
	fmt.Println("Successfully generated submodule wrapper files")
	return nil
}

func runAddAVMInterfaces(ctx context.Context, cmd *cli.Command) error {
	apiVersion := cmd.String("api-version")
	includePreview := cmd.Bool("include-preview")
	targetDir := "."
	if cmd.NArg() > 0 {
		targetDir = cmd.Args().First()
	}

	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	if err := os.Chdir(targetDir); err != nil {
		return fmt.Errorf("failed to change to directory %s: %w", targetDir, err)
	}
	defer os.Chdir(originalDir)

	finalResourceType, err := inferResourceTypeFromMainTf()
	if err != nil {
		return fmt.Errorf("failed to infer resource type from main.tf: %w\nEnsure main.tf exists in %s", err, targetDir)
	}

	var rs *schema.ResourceSchema
	if finalResourceType != "" {
		loaded, err := bicepdata.LoadResource(ctx, finalResourceType, apiVersion, includePreview, nil)
		if err != nil {
			return fmt.Errorf("failed to load resource: %w", err)
		}
		rs, err = schema.ConvertResource(loaded)
		if err != nil {
			return fmt.Errorf("failed to convert resource: %w", err)
		}
	}

	if err := terraform.GenerateInterfacesFile(finalResourceType, rs, "."); err != nil {
		return fmt.Errorf("failed to generate AVM interfaces: %w", err)
	}

	fmt.Println("Successfully generated main.interfaces.tf")
	return nil
}
