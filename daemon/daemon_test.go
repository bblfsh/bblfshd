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

	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/protocol"

	"github.com/bblfsh/bblfshd/runtime"

	"github.com/stretchr/testify/require"
)

func TestDaemonState(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	pool := s.Current()
	require.Len(pool, 1)
	require.NotNil(pool["python"])
}

func TestDaemonParse_MockedDriverParallelClients(t *testing.T) {
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

func buildMockedDaemon(t *testing.T) (*Daemon, string) {
	require := require.New(t)

	dir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	r := runtime.NewRuntime(dir)
	err = r.Init()
	require.NoError(err)

	d := NewDaemon("foo", r)

	dp := NewDriverPool(func() (Driver, error) {
		return newEchoDriver(), nil
	})

	err = dp.Start()
	require.NoError(err)

	d.pool["python"] = dp

	return d, dir
}
