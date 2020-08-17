package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"

	"github.com/olekukonko/tablewriter"
)

const (
	StatusCommandDescription = "List all the pools of driver instances running"
	StatusCommandHelp        = StatusCommandDescription + "\n\n" +
		"The drivers are started on-demand based on the load of the server, \n" +
		"this driver instances are organized in pools by language.\n\n" +
		"This command prints a list of the pools running on the daemon, with \n" +
		"the number of requests success and failed, the number of instances \n" +
		"current and desired, the number of request waiting to be handle and \n" +
		"the drivers existed with with a non-zero code."
)

type StatusCommand struct {
	ControlCommand
}

func (c *StatusCommand) Execute(args []string) error {
	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.DriverPoolStates(context.Background(), &protocol.DriverPoolStatesRequest{})
	if err != nil {
		return err
	}

	if err == nil && len(r.Errors) == 0 {
		daemonStatusToText(r)
		return nil
	}

	return err
}

func daemonStatusToText(r *protocol.DriverPoolStatesResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Language", "Success/Failed", "State/Desired", "Waiting", "Exited"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for language, s := range r.State {
		line := fmt.Sprintf("%s\t%d/%d\t%d/%d\t%d\t%d", language,
			s.Success, s.Errors,
			s.Running, s.Wanted, s.Waiting, s.Exited,
		)
		table.Append(strings.Split(line, "\t"))
	}

	table.Render()
	fmt.Printf("Response time %s\n", r.Elapsed)
}
