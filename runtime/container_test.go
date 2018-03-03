package runtime

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"

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
	require := require.New(s.T())

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)
	s.RuntimePath = tmpDir

	rt := NewRuntime(s.RuntimePath)
	s.Runtime = rt
	err = rt.Init()
	require.NoError(err)

	d, err := NewDriverImage(FixtureReference)
	require.NoError(err)
	s.Image = d

	_, err = rt.InstallDriver(d, false)
	require.NoError(err)
}

func (s *ContainerSuite) TearDownSuite() {
	require := require.New(s.T())
	require.NoError(os.RemoveAll(s.RuntimePath))
}

func (s *ContainerSuite) TestContainer_Run() {
	require := require.New(s.T())

	p := &Process{
		Args:   []string{"/bin/ls"},
		Stdout: os.Stdout,
	}

	c, err := s.Runtime.Container("run", s.Image, p, nil)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
}

func (s *ContainerSuite) TestContainer_StartStopStart() {
	require := require.New(s.T())
	p := &Process{
		Args:   []string{"/bin/sleep", "5m"},
		Stdout: os.Stdout,
	}

	c, err := s.Runtime.Container("1", s.Image, p, nil)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	time.Sleep(100 * time.Millisecond)
	err = c.Stop()
	require.NoError(err)

	p = &Process{
		Args:   []string{"/bin/sleep", "5m"},
		Stdout: os.Stdout,
	}

	c, err = s.Runtime.Container("2", s.Image, p, nil)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)
	time.Sleep(100 * time.Millisecond)

	err = c.Stop()
	require.NoError(err)
}

func (s *ContainerSuite) TestContainer_StartWait() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/ls"},
		Stdout: out,
	}

	c, err := s.Runtime.Container("wait", s.Image, p, nil)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.NoError(err)

	require.Equal("bin\ndev\netc\nhome\nopt\nproc\nroot\ntmp\nusr\nvar\n", out.String())
}

func (s *ContainerSuite) TestContainer_StartWaitExit1() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/false"},
		Stdout: out,
	}

	c, err := s.Runtime.Container("wait-exit", s.Image, p, nil)
	require.NoError(err)

	err = c.Start()
	require.NoError(err)

	err = c.Wait()
	require.Error(err)

	require.Equal("", out.String())
}

func (s *ContainerSuite) TestContainer_StartFailure() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/non-existent"},
		Stdout: out,
	}

	c, err := s.Runtime.Container("start-failure", s.Image, p, nil)
	require.NoError(err)

	err = c.Start()
	require.Error(err)
}

func (s *ContainerSuite) TestContainer_Env() {
	require := require.New(s.T())

	out := bytes.NewBuffer(nil)

	p := &Process{
		Args:   []string{"/bin/env"},
		Stdout: out,
	}

	c, err := s.Runtime.Container("env", s.Image, p, nil)
	require.NoError(err)

	err = c.Run()
	require.NoError(err)
	require.Equal("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\nHOME=/root\n", out.String())
}
