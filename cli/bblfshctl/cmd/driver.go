package cmd

const (
	DriverCommandDescription = "Manage drivers: install, remove and list"
	DriverCommandHelp        = DriverCommandDescription
)

type DriverCommand struct {
	ControlCommand
}

func (*DriverCommand) Execute([]string) error {
	return nil
}
