package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bblfsh/server/daemon/protocol"

	"github.com/olekukonko/tablewriter"
)

const InstancesCommandDescription = "prints the status for each driver instances running on the daemon."

type InstancesCommand struct {
	GRPCCommand
}

func (c *InstancesCommand) Execute(args []string) error {
	if err := c.GRPCCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.DriverInstanceStates(context.Background(), &protocol.DriverInstanceStatesRequest{})
	if err != nil {
		return err
	}

	instancesStatusToText(r)
	return nil
}

func instancesStatusToText(r *protocol.DriverInstanceStatesResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Instance ID", "Driver", "Status", "Created", "PIDs"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, s := range r.State {
		var pids []string
		for _, pid := range s.Processes {
			pids = append(pids, fmt.Sprintf("%d", pid))
		}

		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s",
			s.ID[:10], s.Image,
			s.Status, time.Since(s.Created), pids,
		)
		table.Append(strings.Split(line, "\t"))
	}

	table.Render()
	fmt.Printf("Response time %s\n", r.Elapsed)
}
