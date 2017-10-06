package main

import (
	"fmt"
	"os"

	"github.com/bblfsh/server/cli/bblfshctl/cmd"

	"github.com/jessevdk/go-flags"
)

var (
	version = "undefined"
	build   = "undefined"
)

func main() {
	parser := flags.NewNamedParser("bblfshctl", flags.Default)
	parser.AddCommand("status", cmd.StatusCommandDescription, "", &cmd.StatusCommand{})
	parser.AddCommand("instances", cmd.InstancesCommandDescription, "", &cmd.InstancesCommand{})
	parser.AddCommand("drivers", cmd.DriversCommandDescription, "", &cmd.DriversCommand{})
	parser.AddCommand("parse", cmd.ParseCommandDescription, "", &cmd.ParseCommand{})

	if _, err := parser.Parse(); err != nil {
		if _, ok := err.(*flags.Error); ok {
			parser.WriteHelp(os.Stdout)
			fmt.Printf("\nBuild information\n  commit: %s\n  date: %s\n", version, build)
		}

		os.Exit(1)
	}
}
