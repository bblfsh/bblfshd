package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bblfsh/server/runtime"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/protocol"
)

type Driver interface {
	Start() error
	Stop() error
	Service() protocol.ProtocolServiceClient
}

type DriverInstance struct {
	ctx  context.Context
	p    *runtime.Process
	c    runtime.Container
	i    runtime.DriverImage
	conn *grpc.ClientConn
	srv  protocol.ProtocolServiceClient
	tmp  string
}

const (
	DriverBinary      = "/opt/driver/bin/driver"
	GRPCSocket        = "rpc.sock"
	TmpPathPattern    = "/tmp/%s"
	ConnectionTimeout = 5 * time.Second
)

type Options struct {
	Verbosity string
}

// NewDriverInstance represents a running Driver in the runtime. Its holds the
// container and the connection to the internal grpc server.
func NewDriverInstance(r *runtime.Runtime, i runtime.DriverImage, o *Options) (*DriverInstance, error) {
	p := &runtime.Process{
		Args: []string{
			DriverBinary,
			"--verbose", o.Verbosity,
			"--network", "unix",
			"--address", fmt.Sprintf(TmpPathPattern, GRPCSocket),
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	id := runtime.NewULID()
	tmp := filepath.Join(r.Root, fmt.Sprintf(TmpPathPattern, id))

	f := func(containerID string) *configs.Config {
		cfg := runtime.ContainerConfigFactory(containerID)
		cfg.Mounts = append(cfg.Mounts, &configs.Mount{
			Source:      tmp,
			Destination: "/tmp/",
			Device:      "bind",
			Flags:       syscall.MS_BIND | syscall.MS_REC | syscall.MS_NOSUID,
			PremountCmds: []configs.Command{
				{Path: "mkdir", Args: []string{"-p", tmp}},
			},
		})

		return cfg
	}

	logrus.Debugf("creating container for %s", i.Name())
	c, err := r.Container(id.String(), i, p, f)
	if err != nil {
		return nil, err
	}

	return &DriverInstance{
		ctx: context.Background(),
		tmp: tmp,
		p:   p,
		c:   c,
		i:   i,
	}, nil
}

func (i *DriverInstance) Stop() error {
	return i.c.Stop()
}

func (i *DriverInstance) Start() error {
	logrus.Debugf("starting up container %s (%s)", i.i.Name(), i.c.ID())
	if err := i.c.Start(); err != nil {
		logrus.Errorf("error starting container %s (%s): %s", i.i.Name(), i.c.ID(), err)
		return err
	}

	if err := i.dial(); err != nil {
		return err
	}

	if err := i.loadVersion(); err != nil {
		return err
	}

	logrus.Infof("driver started %s (%s)", i.i.Name(), i.c.ID())
	return nil
}

func (i *DriverInstance) dial() error {
	addr := filepath.Join(i.tmp, GRPCSocket)
	conn, err := grpc.Dial(addr,
		grpc.WithDialer(func(addr string, t time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, t)
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(ConnectionTimeout),
		grpc.WithInsecure(),
	)

	i.conn = conn
	i.srv = protocol.NewProtocolServiceClient(conn)
	return err
}

func (i *DriverInstance) loadVersion() error {
	_, err := i.srv.Version(context.Background(), &protocol.VersionRequest{})
	if err != nil {
		return err
	}

	return nil
}
func (i *DriverInstance) Service() protocol.ProtocolServiceClient {
	return i.srv
}
