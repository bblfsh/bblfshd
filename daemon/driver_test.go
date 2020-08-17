package daemon

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"github.com/bblfsh/bblfshd/v2/runtime"

	"github.com/stretchr/testify/require"
)

func init() {
	runtime.Bootstrap()
}

func TestNewDriver(t *testing.T) {
	require := require.New(t)

	run, image, path := NewRuntime(t)
	defer os.RemoveAll(path)

	_, err := run.InstallDriver(image, false)
	require.NoError(err)

	i, err := NewDriverInstance(run, "foo", image, &Options{
		LogLevel:  "debug",
		LogFormat: "text",
	})

	require.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err = i.Start(ctx)
	require.NoError(err)

	time.Sleep(50 * time.Millisecond)

	err = i.Stop()
	require.NoError(err)
}

func TestDriverInstance_State(t *testing.T) {
	require := require.New(t)

	run, image, path := NewRuntime(t)
	defer os.RemoveAll(path)

	i, err := NewDriverInstance(run, "foo", image, &Options{
		LogLevel:  "debug",
		LogFormat: "text",
	})

	require.NoError(err)

	state, err := i.State()
	require.NoError(err)
	require.Equal(protocol.Stopped, state.Status)
	require.Len(state.Processes, 0)
	require.True(state.Created.IsZero())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err = i.Start(ctx)
	require.NoError(err)
	defer func() {
		err = i.Stop()
		require.NoError(err)
	}()

	time.Sleep(50 * time.Millisecond)

	state, err = i.State()
	require.NoError(err)
	require.Equal(protocol.Running, state.Status)
	require.Len(state.Processes, 2)
	require.False(state.Created.IsZero())
}

func NewRuntime(t *testing.T) (*runtime.Runtime, runtime.DriverImage, string) {
	IfNetworking(t)

	require := require.New(t)

	dir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	run := runtime.NewRuntime(dir)
	err = run.Init()
	require.NoError(err)

	image, err := runtime.NewDriverImage("docker://bblfsh/python-driver:experimental")
	require.NoError(err)

	_, err = run.InstallDriver(image, false)
	require.NoError(err)

	return run, image, dir
}

func IfNetworking(t *testing.T) {
	if len(os.Getenv("TEST_NETWORKING")) != 0 {
		return
	}

	t.Skip("skipping network test use TEST_NETWORKING to run this test")
}
