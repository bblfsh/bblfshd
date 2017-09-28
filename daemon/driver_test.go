package daemon

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/bblfsh/server/runtime"

	"github.com/stretchr/testify/require"
)

func init() {
	runtime.Bootstrap()
}

func TestNewDriver(t *testing.T) {
	r := require.New(t)

	dir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	r.Nil(err)
	defer func() {
		err := os.RemoveAll(dir)
		r.Nil(err)
	}()

	run := runtime.NewRuntime(dir)
	err = run.Init()
	r.Nil(err)

	image, err := runtime.NewDriverImage("docker-daemon:bblfsh/python-driver:dev-0b6ddb4-dirty")
	r.Nil(err)

	err = run.InstallDriver(image, false)
	r.Nil(err)
	i, err := NewDriverInstance(run, "foo", image, &Options{
		LogLevel:  "debug",
		LogFormat: "text",
	})
	r.Nil(err)
	err = i.Start()
	r.NoError(err)

	time.Sleep(5 * time.Second)

	err = i.Stop()
	r.Nil(err)
}
