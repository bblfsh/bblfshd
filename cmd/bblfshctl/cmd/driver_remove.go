package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/bblfsh/bblfshd/daemon/protocol"
)

const (
	DriverRemoveCommandDescription = "Removes a new driver for the given language"
	DriverRemoveCommandHelp        = DriverRemoveCommandDescription
)

type DriverRemoveCommand struct {
	Args struct {
		Language string `positional-arg-name:"language" description:"language supported by the driver"`
	} `positional-args:"yes"`

	DriverCommand
}

func (c *DriverRemoveCommand) Execute(args []string) error {
	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.RemoveDriver(context.Background(), &protocol.RemoveDriverRequest{
		Language: c.Args.Language,
	})

	if err == nil && len(r.Errors) == 0 {
		return nil
	}

	for _, e := range r.Errors {
		fmt.Fprintf(os.Stderr, "Error, %s\n", e)
	}

	return err
}
