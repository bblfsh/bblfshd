package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
)

const (
	DriverRemoveCommandDescription = "Removes the driver for the specified language"
	DriverRemoveCommandHelp        = DriverRemoveCommandDescription
)

type DriverRemoveCommand struct {
	Args struct {
		Language string `positional-arg-name:"language" description:"language supported by the driver"`
	} `positional-args:"yes"`

	All bool `long:"all" description:"removes all the installed drivers"`

	DriverCommand
}

func (c *DriverRemoveCommand) Execute(args []string) error {
	ctx := context.Background()
	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	langs := []string{c.Args.Language}
	if c.All {
		r, err := c.srv.DriverStates(ctx, &protocol.DriverStatesRequest{})
		if err != nil || len(r.Errors) > 0 {
			for _, e := range r.Errors {
				fmt.Fprintf(os.Stderr, "Error, %s\n", e)
			}

			return err
		}

		langs = make([]string, len(r.State))
		for i, s := range r.State {
			langs[i] = s.Language
		}
	}

	for _, lang := range langs {
		r, err := c.srv.RemoveDriver(ctx, &protocol.RemoveDriverRequest{Language: lang})
		if err != nil {
			return err
		} else if len(r.Errors) != 0 {
			for _, e := range r.Errors {
				fmt.Fprintf(os.Stderr, "Error, %s\n", e)
			}
			return fmt.Errorf("driver remove failed: %v", r.Errors)
		}
	}
	return nil
}
