package runtime

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerRun(t *testing.T) {
	require := require.New(t)

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)
	defer func() { require.NoError(os.RemoveAll(tmpDir)) }()

	rt := NewRuntime(tmpDir)
	err = rt.Init()
	require.NoError(err)

	d, err := NewDriverImage("//busybox:latest")
	require.NoError(err)

	err = rt.InstallDriver(d, false)
	require.NoError(err)

	p := &Process{
		Args: []string{"/bin/ls"},
	}

	c, err := rt.Container(d, p)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
}
