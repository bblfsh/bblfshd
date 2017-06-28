package server

import (
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"github.com/bblfsh/server/runtime"
	"google.golang.org/grpc"
)

type GRPCServer struct {
	*Server
}

func NewGRPCServer(r *runtime.Runtime, transport string) *GRPCServer {
	server := NewServer(r)
	server.Transport = transport
	return &GRPCServer{server}
}

func (s *GRPCServer) Serve(listener net.Listener) error {
	grpcServer := grpc.NewServer()

	logrus.Debug("registering gRPC service")
	protocol.RegisterProtocolServiceServer(
		grpcServer,
		protocol.NewProtocolServiceServer(),
	)

	protocol.DefaultParser = s.Server

	logrus.Info("starting gRPC server")
	return grpcServer.Serve(listener)
}
