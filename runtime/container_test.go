package runtime

import (
	"bytes"
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

func TestContainerStartWait(t *testing.T) {
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

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/ls"},
		Stdout: out,
	}

	c, err := rt.Container(d, p)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.NoError(err)

	require.Equal("bin\ndev\netc\nhome\nproc\nroot\nsys\ntmp\nusr\nvar\n", out.String())
}

func TestContainerStartWaitExit1(t *testing.T) {
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

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/false"},
		Stdout: out,
	}

	c, err := rt.Container(d, p)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.Error(err)

	require.Equal("", out.String())
}

func TestContainerStartFailure(t *testing.T) {
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

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/non-existent"},
		Stdout: out,
	}

	c, err := rt.Container(d, p)
	require.NoError(err)

	err = c.Start()
	require.Error(err)
}

func TestContainerEnv(t *testing.T) {
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

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/env"},
		Stdout: out,
	}

	c, err := rt.Container(d, p)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
	require.Equal("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\nHOME=/root\n", out.String())
}
