package daemon

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
	"gopkg.in/bblfsh/sdk.v2/uast"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
)

func TestServiceParse(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewServiceV2(d)
	resp, err := s.Parse(context.Background(), &protocol2.ParseRequest{
		Filename: "foo.py", Content: "foo",
	})
	require.NoError(err)
	ast, err := resp.Nodes()
	require.NoError(err)
	obj, ok := ast.(nodes.Object)
	require.True(ok)
	require.Equal("foo", uast.TokenOf(obj))
	require.Equal(resp.Language, "python")
}

func TestServiceVersion(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t)
	defer os.RemoveAll(tmp)

	s := NewService(d)
	resp := s.Version(&protocol1.VersionRequest{})
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

func TestService_SupportedLanguages(t *testing.T) {
	require := require.New(t)

	d, tmp := buildMockedDaemon(t, newMockDriverImage("language-1"), newMockDriverImage("language-2"))
	defer os.RemoveAll(tmp)

	s := NewService(d)
	languages := s.SupportedLanguages(&protocol1.SupportedLanguagesRequest{})
	require.Len(languages.Errors, 0)
	require.Len(languages.Languages, 2)

	supportedLanguages := make([]string, 2)
	for i, lang := range languages.Languages {
		supportedLanguages[i] = lang.Name
	}

	require.Contains(supportedLanguages, "language-1")
	require.Contains(supportedLanguages, "language-2")
}
