package server

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/bblfsh/server/runtime"

	"github.com/bblfsh/sdk/protocol"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	t.Skip("cannot fetch image yet")

	require := require.New(t)

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	defer func() { require.NoError(os.RemoveAll(tmpDir)) }()

	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r)
	img := DefaultDriverImageReference("", "python")
	err = s.AddDriver("python", img)
	require.NoError(err)

	resp := s.ParseUAST(&protocol.ParseUASTRequest{
		Content: "import foo",
	})
	require.NoError(err)
	require.NotNil(resp)

	require.NoError(s.Close())
}
