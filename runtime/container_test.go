package runtime

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ContainerSuite struct {
	suite.Suite
	RuntimePath string
	Runtime     *Runtime
	Image       DriverImage
}

func TestContainerSuite(t *testing.T) {
	suite.Run(t, new(ContainerSuite))
}

func (s *ContainerSuite) SetupSuite() {
	IfNetworking(s.T())
	require := require.New(s.T())

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)
	s.RuntimePath = tmpDir

	rt := NewRuntime(s.RuntimePath)
	s.Runtime = rt
	err = rt.Init()
	require.NoError(err)

	d, err := NewDriverImage("//busybox:latest")
	require.NoError(err)
	s.Image = d

	err = rt.InstallDriver(d, false)
	require.NoError(err)
}

func (s *ContainerSuite) TearDownSuite() {
	require := require.New(s.T())
	require.NoError(os.RemoveAll(s.RuntimePath))
}

func (s *ContainerSuite) TestContainerRun() {
	require := require.New(s.T())

	p := &Process{
		Args: []string{"/bin/ls"},
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
}

func (s *ContainerSuite) TestContainerStartWait() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/ls"},
		Stdout: out,
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.NoError(err)

	require.Equal("bin\ndev\netc\nhome\nproc\nroot\nsys\ntmp\nusr\nvar\n", out.String())
}

func (s *ContainerSuite) TestContainerStartWaitExit1() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/false"},
		Stdout: out,
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.Error(err)

	require.Equal("", out.String())
}

func (s *ContainerSuite) TestContainerCloseStdoutOnExit() {
	require := require.New(s.T())

	outr, outw := io.Pipe()

	p := &Process{
		Args:   []string{"/bin/true"},
		Stdout: outw,
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	done := make(chan struct{})
	go func() {
		b := make([]byte, 1)
		_, err := outr.Read(b)
		require.Error(err)
		close(done)
	}()

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.NoError(err)

	<-done
}

func (s *ContainerSuite) TestContainerStartFailure() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/non-existent"},
		Stdout: out,
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	err = c.Start()
	require.Error(err)
}

func (s *ContainerSuite) TestContainerEnv() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/env"},
		Stdout: out,
	}

	c, err := s.Runtime.Container(s.Image, p)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
	require.Equal("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\nHOME=/root\n", out.String())
}
