package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bblfsh/server/runtime"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/protocol"
)

type Driver interface {
	Start() error
	Stop() error
	Status() (libcontainer.Status, error)
	Service() protocol.ProtocolServiceClient
}

// DriverInstance represents an instance of a driver.
type DriverInstance struct {
	Language  string
	Process   *runtime.Process
	Container runtime.Container
	Image     runtime.DriverImage

	ctx  context.Context
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
	LogLevel  string
	LogFormat string
}

// NewDriverInstance represents a running Driver in the runtime. Its holds the
// container and the connection to the internal grpc server.
func NewDriverInstance(r *runtime.Runtime, lang string, i runtime.DriverImage, o *Options) (*DriverInstance, error) {
	id := strings.ToLower(runtime.NewULID().String())
	p := &runtime.Process{
		Args: []string{
			DriverBinary,
			"--log-level", o.LogLevel,
			"--log-format", o.LogFormat,
			"--log-fields", logFields(id, lang),
			"--network", "unix",
			"--address", fmt.Sprintf(TmpPathPattern, GRPCSocket),
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

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

	c, err := r.Container(id, i, p, f)
	if err != nil {
		return nil, err
	}

	return &DriverInstance{
		Language:  lang,
		Process:   p,
		Container: c,
		Image:     i,

		ctx: context.Background(),
		tmp: tmp,
	}, nil
}

// Start starts a container and connects to it.
func (i *DriverInstance) Start() error {
	if err := i.Container.Start(); err != nil {
		return err
	}

	if err := i.dial(); err != nil {
		return err
	}

	if err := i.loadVersion(); err != nil {
		return err
	}

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

func (i *DriverInstance) Status() (libcontainer.Status, error) {
	return i.Container.Status()
}

// Stop stops the inner running container.
func (i *DriverInstance) Stop() error {
	return i.Container.Stop()
}

// Service returns the client using the grpc connection.
func (i *DriverInstance) Service() protocol.ProtocolServiceClient {
	return i.srv
}

func logFields(containerID, language string) string {
	js, _ := json.Marshal(map[string]string{
		"id":       containerID,
		"language": language,
	})

	return string(js)
}
