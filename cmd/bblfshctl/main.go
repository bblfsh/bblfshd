package main

import (
	"fmt"
	"os"

	"github.com/bblfsh/bblfshd/v2/cmd/bblfshctl/cmd"

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
		// go-flags won't propagate DriverCommand flags to sub-commands,
		// so we will expect all flags to be passed to the sub-command itself (c1 c2 --x),
		// and not to the parent (c1 --x c2)
		&struct{}{},
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
			fmt.Printf("\nBuild information\n  commit: %s\n  date: %s\n", version, build)
			os.Exit(1)
		}
	}
}
