package core

import "github.com/opencontainers/runc/libcontainer"

type Command interface {
	Run() error
	Start() error
	Wait() error
}

type containerCommand struct {
	container libcontainer.Container
	process   *libcontainer.Process
}

func (c *containerCommand) Start() error {
	if err := c.container.Run(c.process); err != nil {
		c.container.Destroy()
		return err
	}

	return nil
}

func (c *containerCommand) Wait() error {
	if _, err := c.process.Wait(); err != nil {
		return err
	}

	c.container.Destroy()
	return nil
}

func (c *containerCommand) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}
