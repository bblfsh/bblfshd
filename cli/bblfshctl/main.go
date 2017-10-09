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
	parser.AddCommand("status",
		cmd.StatusCommandDescription, cmd.StatusCommandHelp,
		&cmd.StatusCommand{},
	)

	parser.AddCommand("instances",
		cmd.InstancesCommandDescription, cmd.InstancesCommandHelp,
		&cmd.InstancesCommand{},
	)

	parser.AddCommand("parse",
		cmd.ParseCommandDescription, cmd.ParseCommandHelp,
		&cmd.ParseCommand{},
	)

	c, _ := parser.AddCommand("driver",
		cmd.DriverCommandDescription, cmd.DriverCommandHelp,
		&cmd.DriverCommand{},
	)

	c.AddCommand("list",
		cmd.DriverListCommandDescription, cmd.DriverListCommandHelp,
		&cmd.DriverListCommand{},
	)

	c.AddCommand("install",
		cmd.DriverInstallCommandDescription, cmd.DriverInstallCommandHelp,
		&cmd.DriverInstallCommand{},
	)

	c.AddCommand("remove",
		cmd.DriverRemoveCommandDescription, cmd.DriverRemoveCommandHelp,
		&cmd.DriverRemoveCommand{},
	)

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println()
			parser.WriteHelp(os.Stdout)
			fmt.Printf("\nBuild information\n  commit: %s\n  date:%s\n", version, build)
			os.Exit(1)
		}
	}
}
