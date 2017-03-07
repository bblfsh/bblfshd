package runtime

import (
	"os"

	"github.com/opencontainers/runc/libcontainer"
)

// Process defines the process to be executed inside of a container.
type Process libcontainer.Process

// Container represent a container created from a driver image.
type Container interface {
	// Returns the ID of the container
	ID() string
	// Returns the current status of the container.
	Status() (libcontainer.Status, error)
	// State returns the current container's state information.
	State() (*libcontainer.State, error)
	// Returns the PIDs inside this container. The PIDs are in the namespace of the calling process.
	Processes() ([]int, error)
	// Signal sends the provided signal code to all the process in the container.
	Signal(sig os.Signal) error

	Command
}

// Command represents the main command of a container.
type Command interface {
	// Run starts the specified command and waits for it to complete.
	Run() error
	// Start starts the specified command but does not wait for it to complete.
	// The Wait method will return the exit code and release associated
	// resources once the command exits.
	Start() error
	// Wait waits for the command to exit. It must have been started by Start.
	Wait() error
}

func newContainer(c libcontainer.Container, p *Process) Container {
	cp := libcontainer.Process(*p)
	return &container{
		Container: c,
		process:   &cp,
	}
}

type container struct {
	libcontainer.Container
	process *libcontainer.Process
}

func (c *container) Start() error {
	if err := c.Container.Run(c.process); err != nil {
		c.Container.Destroy()
		return err
	}

	return nil
}

func (c *container) Wait() error {
	if _, err := c.process.Wait(); err != nil {
		return err
	}

	c.Container.Destroy()
	return nil
}

func (c *container) Run() error {
	if err := c.Start(); err != nil {
		return err
	}

	return c.Wait()
}

func (c *container) Signal(sig os.Signal) error {
	return c.Container.Signal(sig, true)
}
