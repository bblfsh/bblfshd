package runtime

import (
	"io"
	"os"

	"github.com/opencontainers/image-spec/specs-go/v1"
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

func newContainer(c libcontainer.Container, p *Process, imageDesc *v1.Image) Container {
	cp := libcontainer.Process(*p)
	return &container{
		Container:       c,
		process:         &cp,
		imageDescriptor: imageDesc,
	}
}

type container struct {
	libcontainer.Container
	process         *libcontainer.Process
	imageDescriptor *v1.Image
}

func (c *container) Start() error {
	env := make([]string, len(c.imageDescriptor.Config.Env))
	copy(env, c.imageDescriptor.Config.Env)
	c.process.Env = append(env, c.process.Env...)

	if err := c.Container.Run(c.process); err != nil {
		_ = c.Container.Destroy()
		return err
	}

	return nil
}

func (c *container) Wait() error {
	var derr error
	defer func() {
		derr = c.Container.Destroy()
		streams := []interface{}{
			c.process.Stdin,
			c.process.Stdout,
			c.process.Stderr,
		}
		for _, s := range streams {
			if c, ok := s.(io.Closer); ok {
				if err := c.Close(); err != nil && derr != nil {
					derr = err
				}
			}
		}
	}()

	if _, err := c.process.Wait(); err != nil {
		return err
	}

	return derr
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
