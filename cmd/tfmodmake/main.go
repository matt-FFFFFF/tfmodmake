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
		Usage:   "Generate Terraform modules from OpenAPI specifications",
		Commands: []*cli.Command{
			GenCommand(),
			AddCommand(),
			DiscoverCommand(),
		},
		DefaultCommand: "gen",
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
