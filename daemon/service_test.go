package daemon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/bblfsh/sdk.v1/protocol"
)

func TestServiceParse(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewService(d)
	resp := s.Parse(&protocol.ParseRequest{Filename: "foo.py", Content: "foo"})
	require.Len(resp.Errors, 0)
	require.Equal(resp.UAST.Token, "foo")
	require.True(resp.Elapsed.Nanoseconds() > 0)
}

func TestServiceNativeParse(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewService(d)
	resp := s.NativeParse(&protocol.NativeParseRequest{Filename: "foo.py", Content: "foo"})
	require.Len(resp.Errors, 0)
	require.Equal(resp.AST, "foo")
	require.True(resp.Elapsed.Nanoseconds() > 0)
}

func TestServiceVersion(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewService(d)
	resp := s.Version(&protocol.VersionRequest{})
	require.Len(resp.Errors, 0)
	require.Equal(resp.Version, "foo")
}

func TestControlServiceDriverPoolStates(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewControlService(d)
	state := s.DriverPoolStates()
	require.Len(state, 1)
	require.Equal(state["python"].Running, 1)
}

func TestControlServiceDriverInstanceStates(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewControlService(d)
	state, err := s.DriverInstanceStates()
	require.NoError(err)
	require.Len(state, 1)
}
