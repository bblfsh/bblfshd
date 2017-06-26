package main

import (
	"net"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
)

type serverCmd struct {
	Address     string `long:"address" description:"server address to bind to" default:"0.0.0.0:9432"`
	RuntimePath string `long:"runtime-path" description:"runtime path" default:"/tmp/bblfsh-runtime"`
	Transport   string `long:"transport" description:"default transport to fetch driver images (docker, docker-daemon)" default:"docker"`
	JSON        bool   `long:"json" description:"start a JSON REST server instead of a gRPC server"`
}

func (c *serverCmd) Execute(args []string) error {
	logrus.Debugf("binding to %s", c.Address)

	r := runtime.NewRuntime(c.RuntimePath)
	logrus.Debugf("initializing runtime at %s", c.RuntimePath)
	if err := r.Init(); err != nil {
		return err
	}

	if c.JSON {
		return c.serveJSON(r)
	}

	return c.serveGRPC(r)
}

func (c *serverCmd) serveJSON(r *runtime.Runtime) error {
	s := server.NewRESTServer(r, c.Transport)
	logrus.Debug("starting server")
	return s.Serve(c.Address)
}

func (c *serverCmd) serveGRPC(r *runtime.Runtime) error {
	//TODO: add support for unix://
	lis, err := net.Listen("tcp", c.Address)
	if err != nil {
		return err
	}

	s := server.NewGRPCServer(r, c.Transport)

	logrus.Debug("starting server")
	return s.Serve(lis)
}
