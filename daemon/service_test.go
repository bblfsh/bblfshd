package daemon

import (
	"os"
	"testing"
	"time"

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
	require.Equal(resp.Language, "python")
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
	require.Equal(resp.Language, "python")
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

	bdate, err := time.Parse(time.RFC3339, "2019-01-28T16:49:06+01:00")
	require.NoError(err)
	require.Equal(resp.Build, bdate)
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

func TestService_SupportedLanguages(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t, newMockDriverImage("language-1"), newMockDriverImage("language-2"))
	defer os.RemoveAll(tmp)

	s := NewService(d)
	languages := s.SupportedLanguages(&protocol.SupportedLanguagesRequest{})
	require.Len(languages.Errors, 0)
	require.Len(languages.Languages, 2)

	supportedLanguages := make([]string, 2)
	for i, lang := range languages.Languages {
		supportedLanguages[i] = lang.Name
	}

	require.Contains(supportedLanguages, "language-1")
	require.Contains(supportedLanguages, "language-2")
}
