package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"google.golang.org/grpc"
)

type clientCmd struct {
	Address     string `long:"address" description:"server address to connect to" default:"localhost:9432"`
	Standalone  []bool `long:"standalone" description:"run standalone, without server"`
	RuntimePath string `long:"runtime-path" description:"runtime path for standalone mode" default:"/tmp/bblfsh-runtime"`
	ImageRef    string `long:"image" value-name:"image-ref" description:"image reference to use (e.g. docker://bblfsh/python-driver:latest)"`
	Language    string `long:"language" description:"language of the input" default:""`
	LogLevel    string `long:"log-level" description:"log level" default:"debug"`
	Args        struct {
		File string `positional-arg-name:"file" required:"true"`
	} `positional-args:"yes"`
}

func (c *clientCmd) Execute(args []string) error {
	level, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		return err
	}
	logrus.SetLevel(level)

	logrus.Debugf("reading file: %s", c.Args.File)
	content, err := ioutil.ReadFile(c.Args.File)
	if err != nil {
		return err
	}

	run := c.runClient
	if len(c.Standalone) >= 1 {
		run = c.runStandalone
	}

	req := &protocol.ParseUASTRequest{
		Filename: filepath.Base(c.Args.File),
		Language: c.Language,
		Content:  string(content),
	}
	resp, err := run(req)
	if err != nil {
		return err
	}

	prettyPrinter(os.Stdout, resp)
	return nil
}

func (c *clientCmd) runClient(req *protocol.ParseUASTRequest) (*protocol.ParseUASTResponse, error) {
	logrus.Debugf("dialing server at %s", c.Address)
	conn, err := grpc.Dial(c.Address, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	logrus.Debug("instantiating service client")
	client := protocol.NewProtocolServiceClient(conn)

	logrus.Debug("sending request")
	return client.ParseUAST(context.TODO(), req)
}

func (c *clientCmd) runStandalone(req *protocol.ParseUASTRequest) (*protocol.ParseUASTResponse, error) {
	r := runtime.NewRuntime(c.RuntimePath)
	logrus.Debugf("initializing runtime at %s", c.RuntimePath)
	if err := r.Init(); err != nil {
		return nil, err
	}

	img, err := runtime.NewDriverImage(c.ImageRef)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("ensuring driver is installed")
	if err := r.InstallDriver(img, false); err != nil {
		return nil, err
	}

	logrus.Debugf("executing driver")
	drv, err := server.ExecDriver(r, img)
	if err != nil {
		return nil, err
	}

	logrus.Debug("sending ParseUAST request")
	resp := drv.ParseUAST(req)

	logrus.Debug("closing driver")
	return resp, drv.Close()
}

func prettyPrinter(w io.Writer, r *protocol.ParseUASTResponse) error {
	fmt.Fprintln(w, "Status: ", r.Status)
	fmt.Fprintln(w, "Errors: ")
	for _, err := range r.Errors {
		fmt.Fprintln(w, " . ", err)
	}

	if r.UAST != nil {
		fmt.Fprintln(w, "UAST: ")
		fmt.Fprintln(w, r.UAST.String())
	}

	return nil
}
