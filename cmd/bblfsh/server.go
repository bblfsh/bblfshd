package main

import (
	"net"
	"os"
	"strings"

	errors "srcd.works/go-errors.v0"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
)

var (
	ErrInvalidDriverFormat = errors.NewKind("invalid image driver format %s")
)

type serverCmd struct {
	commonCmd
	Address     string `long:"address" description:"server address to bind to" default:"0.0.0.0:9432"`
	RuntimePath string `long:"runtime-path" description:"runtime path" default:"/tmp/bblfsh-runtime"`
	Transport   string `long:"transport" description:"default transport to fetch driver images (docker, docker-daemon)" default:"docker"`
	REST        bool   `long:"rest" description:"start a JSON REST server instead of a gRPC server"`
}

func (c *serverCmd) Execute(args []string) error {
	if err := c.exec(args); err != nil {
		return err
	}
	logrus.Debugf("binding to %s", c.Address)

	r := runtime.NewRuntime(c.RuntimePath)
	logrus.Debugf("initializing runtime at %s", c.RuntimePath)
	if err := r.Init(); err != nil {
		return err
	}

	overrides := make(map[string]string)
	for _, img := range strings.Split(os.Getenv("BBLFSH_DRIVER_IMAGES"), ";") {
		fields := strings.Split(strings.TrimSpace(img), "=")
		if len(fields) != 2 {
			return ErrInvalidDriverFormat.New(img)
		}
		overrides[strings.TrimSpace(fields[0])] = strings.TrimSpace(fields[1])
	}
	if c.REST {
		return c.serveREST(r, overrides)
	}

	return c.serveGRPC(r, overrides)
}

func (c *serverCmd) serveREST(r *runtime.Runtime, overrides map[string]string) error {
	s := server.NewRESTServer(r, overrides, c.Transport)
	logrus.Debug("starting server")
	return s.Serve(c.Address)
}

func (c *serverCmd) serveGRPC(r *runtime.Runtime, overrides map[string]string) error {
	//TODO: add support for unix://
	lis, err := net.Listen("tcp", c.Address)
	if err != nil {
		return err
	}

	s := server.NewGRPCServer(r, overrides, c.Transport)

	logrus.Debug("starting server")
	return s.Serve(lis)
}
