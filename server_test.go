package server

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bblfsh/server/runtime"

	"github.com/bblfsh/sdk/protocol"
	"github.com/bblfsh/sdk/uast"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type echoDriver struct{}

func (d *echoDriver) ParseUAST(req *protocol.ParseUASTRequest) *protocol.ParseUASTResponse {
	return &protocol.ParseUASTResponse{
		Status: protocol.Ok,
		UAST: &uast.Node{
			Token: req.Content,
		},
	}
}

func (d *echoDriver) Close() error {
	return nil
}

func TestNewServerMockedDriverParallelClients(t *testing.T) {
	require := require.New(t)

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	defer func() { require.NoError(os.RemoveAll(tmpDir)) }()

	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	go (&GRPCServer{s, []grpc.ServerOption{}}).Serve(lis)

	time.Sleep(time.Second * 1)

	wg := &sync.WaitGroup{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure(), grpc.WithTimeout(time.Second*2))
		require.NoError(err)
		go func(i int, conn *grpc.ClientConn) {
			client := protocol.NewProtocolServiceClient(conn)
			iwg := &sync.WaitGroup{}
			for j := 0; j < 50; j++ {
				iwg.Add(1)
				go func(i, j int) {
					content := fmt.Sprintf("# -*- python -*-\nimport foo%d_%d", i, j)
					resp, err := client.ParseUAST(context.TODO(), &protocol.ParseUASTRequest{Content: content})
					require.NoError(err)
					require.Equal(protocol.Ok, resp.Status)
					require.Equal(content, resp.UAST.Token)
					iwg.Done()
				}(i, j)
			}
			iwg.Wait()

			err = conn.Close()
			require.NoError(err)
			wg.Done()
		}(i, conn)
	}

	wg.Wait()
	err = s.Close()
	require.NoError(err)
}

func TestDefaultDriverImageReference(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	s.Transport = "docker"
	require.Equal("docker://bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))
	s.Transport = ""
	require.Equal("docker://bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))
	s.Transport = "docker-daemon"
	require.Equal("docker-daemon:bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))

	python_override := make(map[string]string)
	python_override["python"] = "overriden"
	s = NewServer(r, python_override)
	s.Transport = "docker-daemon"
	require.Equal("overriden", s.defaultDriverImageReference("python"))
}

func TestMaxMessageSizeExceeded(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	go (&GRPCServer{s, []grpc.ServerOption{}}).Serve(lis)

	time.Sleep(time.Second * 1)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure(), grpc.WithTimeout(time.Second*2))
	require.NoError(err)
	client := protocol.NewProtocolServiceClient(conn)
	_, err = client.ParseUAST(context.TODO(), &protocol.ParseUASTRequest{Content: bigContent()})
	require.NotNil(err)
	status, _ := status.FromError(err)
	require.Equal(status.Code(), codes.ResourceExhausted)
	err = conn.Close()
	require.NoError(err)
	err = s.Close()
	require.NoError(err)
}

func TestMaxMessageSizeExceededInClient(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	serverOptions := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(8 * 1024 * 1024),
		grpc.MaxSendMsgSize(8 * 1024 * 1024)}

	go (&GRPCServer{s, serverOptions}).Serve(lis)

	time.Sleep(time.Second * 1)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure(), grpc.WithTimeout(time.Second*2))
	require.NoError(err)
	client := protocol.NewProtocolServiceClient(conn)
	_, err = client.ParseUAST(context.TODO(), &protocol.ParseUASTRequest{Content: bigContent()})
	require.NotNil(err)
	status, _ := status.FromError(err)
	require.Equal(status.Code(), codes.ResourceExhausted)
	err = conn.Close()
	require.NoError(err)
	err = s.Close()
	require.NoError(err)
}

func TestMaxMessageSizeExceededInServer(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	go (&GRPCServer{s, []grpc.ServerOption{}}).Serve(lis)

	time.Sleep(time.Second * 1)

	callOptions := []grpc.CallOption{
		grpc.MaxCallRecvMsgSize(8 * 1024 * 1024),
		grpc.MaxCallSendMsgSize(8 * 1024 * 1024)}
	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithInsecure(),
		grpc.WithTimeout(time.Second*2),
		grpc.WithDefaultCallOptions(callOptions...))
	require.NoError(err)
	client := protocol.NewProtocolServiceClient(conn)
	_, err = client.ParseUAST(context.TODO(), &protocol.ParseUASTRequest{Content: bigContent()})
	require.NotNil(err)
	status, _ := status.FromError(err)
	require.Equal(status.Code(), codes.ResourceExhausted)
	err = conn.Close()
	require.NoError(err)
	err = s.Close()
	require.NoError(err)
}

func TestMaxMessageSizeNotExceeded(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	serverOptions := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(8 * 1024 * 1024),
		grpc.MaxSendMsgSize(8 * 1024 * 1024)}
	go (&GRPCServer{s, serverOptions}).Serve(lis)

	time.Sleep(time.Second * 1)

	callOptions := []grpc.CallOption{
		grpc.MaxCallRecvMsgSize(8 * 1024 * 1024),
		grpc.MaxCallSendMsgSize(8 * 1024 * 1024)}
	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithInsecure(),
		grpc.WithTimeout(time.Second*2),
		grpc.WithDefaultCallOptions(callOptions...))
	require.NoError(err)
	client := protocol.NewProtocolServiceClient(conn)
	_, err = client.ParseUAST(context.TODO(), &protocol.ParseUASTRequest{Content: bigContent()})
	require.NoError(err)
	err = conn.Close()
	require.NoError(err)
	err = s.Close()
	require.NoError(err)
}

// Generate a string longer than 4MB, which is the default message limit in GRPC
func bigContent() string {
	content := bytes.NewBufferString("")
	for i := 0; i < 65536; i++ {
		content.WriteString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	}
	return content.String()
}
