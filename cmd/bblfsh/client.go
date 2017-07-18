package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"google.golang.org/grpc"
	"srcd.works/go-errors.v0"
)

var (
	// ErrProfiler happens if a profiler fails
	ErrProfiler = errors.NewKind("Failed to create a % file at %s")
)

type clientCmd struct {
	commonCmd
	Address     string `long:"address" description:"server address to connect to" default:"localhost:9432"`
	Standalone  []bool `long:"standalone" description:"run standalone, without server"`
	RuntimePath string `long:"runtime-path" description:"runtime path for standalone mode" default:"/tmp/bblfsh-runtime"`
	ImageRef    string `long:"image" value-name:"image-ref" description:"image reference to use (e.g. docker://bblfsh/python-driver:latest)"`
	Language    string `long:"language" description:"language of the input" default:""`
	Encoding    string `long:"encoding" description:"encoding used in the source file" default:"UTF8"`
	CPUProfile  string `long:"cpuprofile" description:"path to file where Cpu Profile will be stored" default:""`
	MemProfile  string `long:"memprofile" description:"path to file where Memory Profile will be stored" default:""`
	Args        struct {
		File string `positional-arg-name:"file" required:"true"`
	} `positional-args:"yes"`
	CPUProfileFile *os.File
}

func (c *clientCmd) StartCPUProfileMaybe() error {
	if c.CPUProfile != "" {
		f, err := os.Create(c.CPUProfile)
		if err != nil {
			logrus.Errorf("Failed to create a CpuProfile file at %s, err:%s", c.CPUProfile, err)
			return ErrProfiler.Wrap(err, "CpuProfile", c.CPUProfile)
		}
		c.CPUProfileFile = f
		logrus.Infof("Start CPU profiling, save to %s", c.CPUProfile)
		pprof.StartCPUProfile(f)
	}
	return nil
}
func (c *clientCmd) StopCPUProfile() {
	logrus.Info("Stop CPU profiling")
	pprof.StopCPUProfile()
	if c.CPUProfileFile != nil {
		c.CPUProfileFile.Close()
	}
}

// If profiling enabled though CLI, it saves memory profile to file.
func (c *clientCmd) SaveMemProfileMaybe() error {
	if c.MemProfile != "" {
		f, err := os.Create(c.MemProfile)
		if err != nil {
			logrus.Errorf("Failed to save Heap profile to %s, err:%s", c.MemProfile, err)
			return ErrProfiler.Wrap(err, "MemProfile", c.MemProfile)
		}
		logrus.Infof("Save Heap profile to %s", c.MemProfile)
		pprof.WriteHeapProfile(f)
		f.Close()
	}
	return nil
}

func (c *clientCmd) Execute(args []string) error {
	c.StartCPUProfileMaybe()
	if err := c.exec(args); err != nil {
		return err
	}

	logrus.Debugf("reading file: %s", c.Args.File)
	content, err := ioutil.ReadFile(c.Args.File)
	if err != nil {
		return err
	}

	run := c.runClient
	if len(c.Standalone) >= 1 {
		run = c.runStandalone
	}

	var encoding protocol.Encoding
	switch c.Encoding {
	case "Base64":
		encoding = protocol.Base64
	default:
		encoding = protocol.UTF8
	}

	req := &protocol.ParseUASTRequest{
		Filename: filepath.Base(c.Args.File),
		Language: c.Language,
		Content:  string(content),
		Encoding: encoding,
	}
	resp, err := run(req)
	if err != nil {
		return err
	}

	c.StopCPUProfile()
	c.SaveMemProfileMaybe()

	prettyPrinter(os.Stdout, resp)
	return nil
}

func (c *clientCmd) runClient(req *protocol.ParseUASTRequest) (*protocol.ParseUASTResponse, error) {
	maxMessageSize, err := c.parseMaxMessageSize()
	if err != nil {
		return nil, err
	}

	callOptions := []grpc.CallOption{}
	if maxMessageSize != 0 {
		callOptions = append(callOptions, grpc.MaxCallRecvMsgSize(maxMessageSize))
		callOptions = append(callOptions, grpc.MaxCallSendMsgSize(maxMessageSize))
	}

	logrus.Debugf("dialing server at %s", c.Address)
	conn, err := grpc.Dial(c.Address, grpc.WithInsecure(), grpc.WithDefaultCallOptions(callOptions...))
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
