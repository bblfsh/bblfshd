package daemon

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/protocol"

	"github.com/bblfsh/bblfshd/v2/runtime"
)

// actual date format used in bblfshd is different
const testBuildDate = "2019-01-28T16:49:06+01:00"

func TestDaemonState(t *testing.T) {
	require := require.New(t)

	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	pool := s.Current()
	require.Len(pool, 1)
	require.NotNil(pool["python"])
}

func TestDaemon_InstallDriver(t *testing.T) {
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

func TestDaemon_InstallNonexistentDriver(t *testing.T) {
	require := require.New(t)
	s, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	err := s.InstallDriver("", "docker://list", false)
	require.Error(err, "An error was expected")
	require.Equal("errcode.Errors", reflect.TypeOf(err).String())
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
		i := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			conn, err := grpc.Dial(lis.Addr().String(),
				grpc.WithBlock(),
				grpc.WithInsecure(),
				grpc.WithTimeout(2*time.Second),
			)
			require.NoError(err)
			defer conn.Close()

			client := protocol.NewProtocolServiceClient(conn)
			var iwg sync.WaitGroup
			for j := 0; j < 50; j++ {
				iwg.Add(1)
				j := j
				go func() {
					defer iwg.Done()
					content := fmt.Sprintf("# -*- python -*-\nimport foo%d_%d", i, j)
					resp, err := client.Parse(context.TODO(), &protocol.ParseRequest{Content: content})
					require.NoError(err)
					require.Equal(protocol.Ok, resp.Status, "%s: %v", resp.Status, resp.Errors)
					require.Equal(content, resp.UAST.Token)
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

	parsedBuild, err := time.Parse(time.RFC3339, testBuildDate)
	d := NewDaemon("foo", parsedBuild, r)

	dp := NewDriverPool(func(ctx context.Context) (Driver, error) {
		return newEchoDriver(), nil
	})

	err = dp.Start(context.Background())
	require.NoError(err)

	d.pool["python"] = dp

	return d, dir
}
