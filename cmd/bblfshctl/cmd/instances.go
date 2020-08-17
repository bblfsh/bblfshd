package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"

	"github.com/docker/go-units"
	"github.com/olekukonko/tablewriter"
)

const (
	InstancesCommandDescription = "List the driver instances running on the daemon"
	InstancesCommandHelp        = InstancesCommandDescription
)

type InstancesCommand struct {
	ControlCommand
}

func (c *InstancesCommand) Execute(args []string) error {
	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.DriverInstanceStates(context.Background(), &protocol.DriverInstanceStatesRequest{})
	if err != nil {
		return err
	}

	if err == nil && len(r.Errors) == 0 {
		instancesStatusToText(r)
		return nil
	}

	return err
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
			s.Status,
			units.HumanDuration(time.Since(s.Created)),
			strings.Join(pids, ","),
		)

		table.Append(strings.Split(line, "\t"))
	}

	table.Render()
	fmt.Printf("Response time %s\n", r.Elapsed)
}
