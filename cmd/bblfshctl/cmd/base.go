package cmd

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/bblfsh/bblfshd/daemon/protocol"

	"google.golang.org/grpc"
	sdk "gopkg.in/bblfsh/sdk.v1/protocol"
)

type ControlCommand struct {
	Network string `long:"ctl-network" default:"unix" description:"control server network type"`
	Address string `long:"ctl-address" default:"/var/run/bblfshctl.sock" description:"control server address to connect"`

	conn *grpc.ClientConn
	srv  protocol.ProtocolServiceClient
}

func (c *ControlCommand) Execute(args []string) error {
	var err error
	c.conn, err = dialGRPC(c.Network, c.Address)
	c.srv = protocol.NewProtocolServiceClient(c.conn)
	return err
}

type UserCommand struct {
	Network  string `long:"network" default:"tcp" description:"server network type"`
	Address  string `long:"address" default:"localhost:9432" description:"server address to connect"`
	Endpoint string `long:"endpoint" description:"server address to connect (alias for --address)"`

	conn *grpc.ClientConn
	srv  sdk.ProtocolServiceClient
}

func (c *UserCommand) targetNetwork() string {
	if c.Endpoint == "tcp" || c.Endpoint == "unix" {
		return c.Endpoint
	}
	if c.Network != "" {
		return c.Network
	}
	return "tcp"
}

func (c *UserCommand) targetAddress() string {
	if c.Endpoint != "" && c.Endpoint != "tcp" && c.Endpoint != "unix" {
		return c.Endpoint
	}
	if c.Address != "" {
		return c.Address
	}
	return "localhost:9432"
}

func (c *UserCommand) Execute(args []string) error {
	var err error
	c.conn, err = dialGRPC(c.targetNetwork(), c.targetAddress())
	c.srv = sdk.NewProtocolServiceClient(c.conn)
	return err
}

func dialGRPC(network, address string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(address,
		grpc.WithDialer(func(addr string, t time.Duration) (net.Conn, error) {
			return net.DialTimeout(network, address, t)
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
		grpc.WithInsecure(),
	)
	if err == context.DeadlineExceeded {
		return nil, fmt.Errorf("failed to connect to %s (%s): timeout", address, network)
	}
	return conn, err
}
