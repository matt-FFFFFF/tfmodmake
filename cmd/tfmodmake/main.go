package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"
)

var version = "dev"

func main() {
	cmd := &cli.Command{
		Version: version,
		Name:    "tfmodmake",
		Usage:   "Generate Terraform modules from Azure resource type definitions",
		Commands: []*cli.Command{
			GenCommand(),
			AddCommand(),
			DiscoverCommand(),
			UpdateCommand(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
