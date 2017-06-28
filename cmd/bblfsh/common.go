package main

import (
	"github.com/Sirupsen/logrus"
)

type commonCmd struct {
	LogLevel    string `long:"log-level" description:"log level" default:"debug"`
}

func (c *commonCmd) exec(args []string) error {
	level, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)
	return nil
}

