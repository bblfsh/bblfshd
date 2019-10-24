// +build linux

package runtime

import (
	"os"
	"syscall"
	
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
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
	// Signal sends the provided signal code to the running process in the container.
	Signal(sig os.Signal) error
	// Returns the current config of the container.
	Config() configs.Config
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
	// Stop kills the container.
	Stop() error
}

func newContainer(c libcontainer.Container, p *Process, config *ImageConfig) Container {
	cp := libcontainer.Process(*p)
	return &container{
		Container: c,
		process:   &cp,
		config:    config,
	}
}

type container struct {
	libcontainer.Container
	process *libcontainer.Process
	config  *ImageConfig
}

func (c *container) Start() error {
	env := make([]string, len(c.config.Config.Env))
	copy(env, c.config.Config.Env)
	c.process.Env = append(env, c.process.Env...)

	if err := c.Container.Run(c.process); err != nil {
		_ = c.Container.Destroy()
		return err
	}

	return nil
}

func (c *container) Wait() error {
	_, err := c.process.Wait()
	return err
}

func (c *container) Run() error {
	if err := c.Start(); err != nil {
		return err
	}

	return c.Wait()
}

func (c *container) Stop() error {
	// Running bblfshd as a rootless container requires to use
	// SIGKILL instead of SIGTERM or SIGINT to kill the process.
	// Otherwise it ignores the order
	if err := c.process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	// kills all the remaining processes
	if err := c.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	return c.Destroy()
}

func (c *container) Signal(sig os.Signal) error {
	return c.Container.Signal(sig, true)

}
