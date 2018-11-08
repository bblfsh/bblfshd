// +build linux,cgo

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

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/opencontainers/runc/libcontainer/configs"
	"google.golang.org/grpc"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
)

type ServiceV1 interface {
	// SupportedLanguages uses DefaultService to process the given SupportedLanguagesRequest to get the supported drivers.
	SupportedLanguages(ctx context.Context, in *protocol1.SupportedLanguagesRequest, opts ...grpc.CallOption) (*protocol1.SupportedLanguagesResponse, error)
	// Version uses DefaultVersioner to process the given version request to get the version.
	Version(ctx context.Context, in *protocol1.VersionRequest, opts ...grpc.CallOption) (*protocol1.VersionResponse, error)
}

type Driver interface {
	ID() string
	Start() error
	Stop() error
	Status() (protocol.Status, error)
	State() (*protocol.DriverInstanceState, error)
	Service() ServiceV1
	ServiceV2() protocol2.DriverClient
}

// DriverInstance represents an instance of a driver.
type DriverInstance struct {
	Language  string
	Process   *runtime.Process
	Container runtime.Container
	Image     runtime.DriverImage

	ctx  context.Context
	conn *grpc.ClientConn
	srv1 protocol1.ProtocolServiceClient
	srv2 protocol2.DriverClient
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

// ID returns the container id.
func (i *DriverInstance) ID() string {
	return i.Container.ID()
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
	i.srv1 = protocol1.NewProtocolServiceClient(conn)
	i.srv2 = protocol2.NewDriverClient(conn)
	return err
}

func (i *DriverInstance) loadVersion() error {
	_, err := i.srv1.Version(context.Background(), &protocol1.VersionRequest{})
	if err != nil {
		return err
	}

	return nil
}

// Status returns the current status of the container.
func (i *DriverInstance) Status() (protocol.Status, error) {
	s, err := i.Container.Status()
	return protocol.Status(s), err
}

// State returns the current state of the driver instance.
func (i *DriverInstance) State() (*protocol.DriverInstanceState, error) {
	status, err := i.Status()
	if err != nil {
		return nil, err
	}

	pid, err := i.Container.Processes()
	if err != nil {
		return nil, err
	}

	state, err := i.Container.State()
	if err != nil {
		return nil, err
	}

	return &protocol.DriverInstanceState{
		ID:        i.ID(),
		Image:     i.Image.Name(),
		Status:    status,
		Processes: pid,
		Created:   state.Created,
	}, nil
}

// Stop stops the inner running container.
func (i *DriverInstance) Stop() error {
	return i.Container.Stop()
}

// Service returns the client using the grpc connection.
func (i *DriverInstance) Service() ServiceV1 {
	return i.srv1
}

// ServiceV2 returns the client using the grpc connection.
func (i *DriverInstance) ServiceV2() protocol2.DriverClient {
	return i.srv2
}

func logFields(containerID, language string) string {
	js, _ := json.Marshal(map[string]string{
		"id":       containerID,
		"language": language,
	})

	return string(js)
}
