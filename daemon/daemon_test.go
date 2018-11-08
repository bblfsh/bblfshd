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

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/bblfsh/bblfshd/runtime"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
	"gopkg.in/bblfsh/sdk.v2/uast"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
)

func TestDaemonState(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	pool := s.Current()
	require.Len(pool, 1)
	require.NotNil(pool["python"])
}

func TestDaemonInstallDriver(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	err := s.InstallDriver("go", "docker://bblfsh/go-driver:latest", false)
	require.Nil(err)
	err = s.InstallDriver("go", "docker://bblfsh/go-driver:latest", false)
	require.True(ErrAlreadyInstalled.Is(err))
	err = s.InstallDriver("go", "docker://bblfsh/go-driver:latest", true)
	require.Nil(err)
}

func TestDaemonParse_MockedDriverParallelClients(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(err)
	go d.UserServer.Serve(lis)
	defer func() {
		err = d.Stop()
		require.NoError(err)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i // copy fo the goroutine
		wg.Add(1)
		conn, err := grpc.Dial(lis.Addr().String(),
			grpc.WithBlock(),
			grpc.WithInsecure(),
			grpc.WithTimeout(2*time.Second),
		)

		require.NoError(err)
		go func() {
			defer wg.Done()
			client := protocol2.NewDriverClient(conn)
			var iwg sync.WaitGroup
			for j := 0; j < 50; j++ {
				j := j // copy for the goroutine
				iwg.Add(1)
				go func() {
					defer iwg.Done()
					content := fmt.Sprintf("# -*- python -*-\nimport foo%d_%d", i, j)
					resp, err := client.Parse(context.TODO(), &protocol2.ParseRequest{Content: content})
					require.NoError(err)
					ast, err := resp.Nodes()
					require.NoError(err)
					obj, ok := ast.(nodes.Object)
					require.True(ok)
					require.Equal(content, uast.TokenOf(obj))
				}()
			}
			iwg.Wait()

			err = conn.Close()
			require.NoError(err)
		}()
	}

	wg.Wait()

}

func buildMockedDaemon(t *testing.T, images ...runtime.DriverImage) (*Daemon, string) {
	require := require.New(t)

	dir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	r := runtime.NewRuntime(dir)
	err = r.Init()
	require.NoError(err)

	if images != nil {
		for _, image := range images {
			status, err := r.InstallDriver(image, false)
			require.NotNil(status)
			require.NoError(err)
		}
	}

	d := NewDaemon("foo", r)

	dp := NewDriverPool(func() (Driver, error) {
		return newEchoDriver(), nil
	})

	err = dp.Start()
	require.NoError(err)

	d.pool["python"] = dp

	return d, dir
}
