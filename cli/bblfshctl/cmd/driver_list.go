package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bblfsh/server/daemon/protocol"

	"github.com/bblfsh/server/runtime"
	"github.com/docker/go-units"
	"github.com/olekukonko/tablewriter"
)

const (
	DriverListCommandDescription = "List the installed drivers for each language"
	DriverListCommandHelp        = DriverListCommandDescription
)

type DriverListCommand struct {
	DriverCommand
}

func (c *DriverListCommand) Execute(args []string) error {
	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	r, err := c.srv.DriverStates(context.Background(), &protocol.DriverStatesRequest{})
	if err != nil {
		return err
	}

	if err == nil && len(r.Errors) == 0 {
		driverStatusToText(r)
		return nil
	}

	printErrors(r.Errors)
	return err
}

func driverStatusToText(r *protocol.DriverStatesResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Language", "Image", "Version", "Status", "Created", "OS", "Go", "Native"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, s := range r.State {
		var native []string
		for _, v := range s.NativeVersion {
			native = append(native, fmt.Sprintf("%s", v))
		}

		image, _ := runtime.ParseImageName(s.Reference)

		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s",
			s.Language, image.StringWithinTransport(), s.Version,
			s.Status, units.HumanDuration(time.Since(s.Build)),
			s.OS, s.GoVersion, strings.Join(native, ","),
		)
		table.Append(strings.Split(line, "\t"))
	}

	table.Render()
	fmt.Printf("Response time %s\n", r.Elapsed)
}
