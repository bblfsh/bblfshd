package main

import (
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
	"srcd.works/go-errors.v0"
)

var (
	ErrInvalidDriverFormat = errors.NewKind("invalid image driver format %s")
)

type serverCmd struct {
	commonCmd
	Address      string `long:"address" description:"server address to bind to" default:"0.0.0.0:9432"`
	RuntimePath  string `long:"runtime-path" description:"runtime path" default:"/tmp/bblfsh-runtime"`
	Transport    string `long:"transport" description:"default transport to fetch driver images (docker, docker-daemon)" default:"docker"`
	REST         bool   `long:"rest" description:"start a JSON REST server instead of a gRPC server"`
	Profiler     bool   `long:"profiler" description:"start CPU & memory profiler"`
	ProfilerAddr string `long:"profiler-address" description:"address to bind profiler to, in case of gRPC" default:"0.0.0.0:6062"`
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
		if img = strings.TrimSpace(img); img == "" {
			continue
		}

		fields := strings.Split(img, "=")
		if len(fields) != 2 {
			return ErrInvalidDriverFormat.New(img)
		}

		lang := strings.TrimSpace(fields[0])
		image := strings.TrimSpace(fields[1])
		logrus.Debugf("Overriding image for %s: %s", lang, image)
		overrides[lang] = image
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
	c.startProfilingHTTPServerMaybe(c.ProfilerAddr)
	maxMessageSize, err := c.parseMaxMessageSize()
	if err != nil {
		return err
	}

	//TODO: add support for unix://
	lis, err := net.Listen("tcp", c.Address)
	if err != nil {
		return err
	}

	s := server.NewGRPCServer(r, overrides, c.Transport, maxMessageSize)

	logrus.Debug("starting server")
	return s.Serve(lis)
}

func (c *serverCmd) startProfilingHTTPServerMaybe(addr string) {
	if c.Profiler {
		go func() {
			logrus.Debugf("Started CPU & Heap profiler at %s", addr)
			err := http.ListenAndServe(addr, nil)
			if err != nil {
				logrus.Warnf("Profiler failed to listen and serve at %s, err: %s", addr, err)
			}
		}()
	}
}
