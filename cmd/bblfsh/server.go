package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
	"gopkg.in/src-d/go-errors.v0"
)

var (
	ErrInvalidDriverFormat = errors.NewKind("invalid image driver format %s")
)

type serverCmd struct {
	commonCmd
	Address      string `long:"address" description:"server address to bind to" default:"0.0.0.0:9432"`
	RuntimePath  string `long:"runtime-path" description:"runtime path" default:"/tmp/bblfsh-runtime"`
	Transport    string `long:"transport" description:"default transport to fetch driver images (docker, docker-daemon)" default:"docker"`
	Profiler     bool   `long:"profiler" description:"start CPU & memory profiler"`
	ProfilerAddr string `long:"profiler-address" description:"address to bind profiler to, in case of gRPC" default:"0.0.0.0:6062"`
}

func (c *serverCmd) Execute(args []string) error {
	if err := c.exec(args); err != nil {
		return err
	}
	logrus.Infof("binding to %s", c.Address)

	r := runtime.NewRuntime(c.RuntimePath)
	logrus.Infof("initializing runtime at %s", c.RuntimePath)
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
		logrus.Infof("Overriding image for %s: %s", lang, image)
		overrides[lang] = image
	}

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

	v := fmt.Sprintf("%s (%s)", version, build)
	s := server.NewServer(v, r, overrides)
	s.Transport = c.Transport

	logrus.Debug("starting server")
	return s.Serve(lis, maxMessageSize)
}

func (c *serverCmd) startProfilingHTTPServerMaybe(addr string) {
	if c.Profiler {
		go func() {
			logrus.Infof("Started CPU & Heap profiler at %s", addr)
			err := http.ListenAndServe(addr, nil)
			if err != nil {
				logrus.Warnf("Profiler failed to listen and serve at %s, err: %s", addr, err)
			}
		}()
	}
}
