package cmd

import (
	"net"
	"time"

	"github.com/bblfsh/server/daemon/protocol"

	"google.golang.org/grpc"
)

type GRPCCommand struct {
	Network string `long:"endpoint" default:"unix" description:"control server network type"`
	Address string `long:"address" default:"/var/run/bblfshctl.sock" description:"control server address to connect"`

	conn *grpc.ClientConn
	srv  protocol.ProtocolServiceClient
}

func (c *GRPCCommand) Execute(args []string) error {
	conn, err := grpc.Dial(c.Address,
		grpc.WithDialer(func(addr string, t time.Duration) (net.Conn, error) {
			return net.DialTimeout(c.Network, addr, t)
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
		grpc.WithInsecure(),
	)

	c.conn = conn
	c.srv = protocol.NewProtocolServiceClient(conn)
	return err
}
