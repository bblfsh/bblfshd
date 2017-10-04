package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bblfsh/server/daemon/protocol"

	"github.com/olekukonko/tablewriter"
)

const StatusCommandDescription = "prints the status for each pool of driver instances running on the daemon."

type StatusCommand struct {
	GRPCCommand
}

func (c *StatusCommand) Execute(args []string) error {
	if err := c.GRPCCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.DriverPoolStates(context.Background(), &protocol.DriverPoolStatesRequest{})
	if err != nil {
		return err
	}

	daemonStatusToText(r)
	return nil
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
