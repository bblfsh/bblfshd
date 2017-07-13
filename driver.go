package server

import (
	"bufio"
	"io"

	"github.com/bblfsh/server/runtime"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"github.com/bblfsh/sdk/protocol/driver"
)

// Driver is a client to communicate with a driver. It provides the parser
// interface and is closeable.
type Driver interface {
	protocol.Parser
	io.Closer
}

// ExecDriver executes a new driver using the given runtime and driver image
// and returns a Driver instance for it. The Driver instance returned by this
// method is not thread-safe.
func ExecDriver(r *runtime.Runtime, img runtime.DriverImage) (Driver, error) {
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	errr, errw := io.Pipe()

	p := &runtime.Process{
		Args:   []string{"/opt/driver/bin/driver", "serve"},
		Stdin:  inr,
		Stdout: outw,
		Stderr: errw,
	}

	logrus.Debugf("creating container for %s", img.Name())
	c, err := r.Container(img, p)
	if err != nil {
		_ = inr.Close()
		_ = outr.Close()
		return nil, err
	}

	go func() {
		s := bufio.NewScanner(errr)
		for s.Scan() {
			logrus.Errorf("driver %s (%s) stderr: %s", img.Name(), c.ID(), s.Text())
		}
	}()

	logrus.Debugf("starting up container %s (%s)", img.Name(), c.ID())
	if err := c.Start(); err != nil {
		logrus.Errorf("error starting container %s (%s): %s", img.Name(), c.ID(), err.Error())
		_ = inr.Close()
		_ = outr.Close()
		_ = errr.Close()
		return nil, err
	}
	logrus.Debugf("container started %s (%s)", img.Name(), c.ID())

	go func() {
		if err := c.Wait(); err != nil {
			logrus.Errorf("driver exited with error: %s", err)
		} else {
			logrus.Debug("driver exited without error")
		}
	}()

	client := driver.NewClient(inw, outr)
	return loggingDriver{client}, nil
}

type loggingDriver struct {
	Driver
}

func (d loggingDriver) Parse(req *protocol.ParseRequest) *protocol.ParseResponse {
	logrus.Debugf("sending Parse request: %s", req.String())
	return d.Driver.Parse(req)
}
