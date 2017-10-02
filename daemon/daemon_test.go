package daemon

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bblfsh/server/runtime"
	"github.com/opencontainers/runc/libcontainer"

	"github.com/stretchr/testify/require"
	oldctx "golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/uast"
)

func TestNewServer_Parse(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	resp := s.Parse(&protocol.ParseRequest{Filename: "foo.py", Content: "foo"})
	require.Len(resp.Errors, 0)
	require.Equal(resp.UAST.Token, "foo")
	require.True(resp.Elapsed.Nanoseconds() > 0)
}

func TestNewServer_NativeParse(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	resp := s.NativeParse(&protocol.NativeParseRequest{Filename: "foo.py", Content: "foo"})
	require.Len(resp.Errors, 0)
	require.Equal(resp.AST, "foo")
	require.True(resp.Elapsed.Nanoseconds() > 0)
}

func buildMockedDaemon(t *testing.T) (*Daemon, string) {
	require := require.New(t)

	dir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	r := runtime.NewRuntime(dir)
	err = r.Init()
	require.NoError(err)

	d := NewDaemon("foo", r)

	dp := NewDriverPool(func() (Driver, error) {
		return &echoDriver{}, nil
	})

	err = dp.Start()
	require.NoError(err)

	d.pool["python"] = dp

	return d, dir
}
func TestNewServer_MockedDriverParallelClients(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	go d.Serve(lis)
	defer func() {
		err = d.Stop()
		require.NoError(err)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		conn, err := grpc.Dial(lis.Addr().String(),
			grpc.WithBlock(),
			grpc.WithInsecure(),
			grpc.WithTimeout(2*time.Second),
		)

		require.NoError(err)
		go func(i int, conn *grpc.ClientConn) {
			client := protocol.NewProtocolServiceClient(conn)
			var iwg sync.WaitGroup
			for j := 0; j < 50; j++ {
				iwg.Add(1)
				go func(i, j int) {
					content := fmt.Sprintf("# -*- python -*-\nimport foo%d_%d", i, j)
					resp, err := client.Parse(context.TODO(), &protocol.ParseRequest{Content: content})
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

}

func TestDefaultDriverImageReference(t *testing.T) {
	require := require.New(t)
	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewDaemon("", r)
	s.Transport = "docker"
	require.Equal("docker://bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))
	s.Transport = ""
	require.Equal("docker://bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))
	s.Transport = "docker-daemon"
	require.Equal("docker-daemon:bblfsh/python-driver:latest", s.defaultDriverImageReference("python"))

	s = NewDaemon("", r)
	s.Overrides["python"] = "overriden"
	s.Transport = "docker-daemon"
	require.Equal("overriden", s.defaultDriverImageReference("python"))
}

type echoDriver struct{}

func (d *echoDriver) NativeParse(
	_ oldctx.Context, in *protocol.NativeParseRequest, opts ...grpc.CallOption) (*protocol.NativeParseResponse, error) {
	return &protocol.NativeParseResponse{
		AST: in.Content,
	}, nil
}

func (d *echoDriver) Parse(
	_ oldctx.Context, in *protocol.ParseRequest, opts ...grpc.CallOption) (*protocol.ParseResponse, error) {
	return &protocol.ParseResponse{
		UAST: &uast.Node{
			Token: in.Content,
		},
	}, nil
}

func (d *echoDriver) Version(
	_ oldctx.Context, in *protocol.VersionRequest, opts ...grpc.CallOption) (*protocol.VersionResponse, error) {
	return &protocol.VersionResponse{}, nil
}

func (d *echoDriver) Start() error {
	return nil
}

func (d *echoDriver) Status() (libcontainer.Status, error) {
	return libcontainer.Running, nil
}

func (d *echoDriver) Service() protocol.ProtocolServiceClient {
	return d
}

func (d *echoDriver) Stop() error {
	return nil
}
