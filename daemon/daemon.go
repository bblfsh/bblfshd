// +build linux,cgo

package daemon

import (
	"sync"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
)

// Daemon is a Babelfish server.
type Daemon struct {
	UserServer    *grpc.Server
	ControlServer *grpc.Server

	version string
	runtime *runtime.Runtime
	pool    map[string]*DriverPool
	mutex   sync.Mutex
}

// NewDaemon creates a new server based on the runtime with the given version.
func NewDaemon(version string, r *runtime.Runtime, opts ...grpc.ServerOption) *Daemon {
	d := &Daemon{
		version:       version,
		runtime:       r,
		pool:          make(map[string]*DriverPool),
		UserServer:    grpc.NewServer(opts...),
		ControlServer: grpc.NewServer(),
	}
	registerGRPC(d)
	return d
}

func registerGRPC(d *Daemon) {
	protocol1.DefaultService = NewService(d)
	protocol1.RegisterProtocolServiceServer(d.UserServer, protocol1.NewProtocolServiceServer())

	protocol2.RegisterDriverServer(d.UserServer, NewServiceV2(d))
	protocol.RegisterService(d.ControlServer, NewControlService(d))
}

func (d *Daemon) InstallDriver(language string, image string, update bool) error {
	img, err := runtime.NewDriverImage(image)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}
	if language == "" {
		info, err := img.Inspect()
		if err != nil {
			return err
		}
		if lang, ok := info.Labels["bblfsh.language"]; ok {
			language = lang
		} else {
			return ErrLanguageDetection.New()
		}
	}

	s, err := d.getDriverImage(language)
	if err != nil && !ErrMissingDriver.Is(err) {
		return ErrRuntime.Wrap(err)
	}
	if err == nil {
		if !update {
			return ErrAlreadyInstalled.Wrap(err, language, image)
		}
		// TODO: the old driver should be removed only after a successful install of the new one
		if err := d.runtime.RemoveDriver(s); err != nil {
			return err
		}
	}

	_, err = d.runtime.InstallDriver(img, update)
	if err != nil {
		return err
	}

	logrus.Infof("driver %s installed %q", language, img.Name())
	return nil
}

func (d *Daemon) RemoveDriver(language string) error {
	img, err := d.getDriverImage(language)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}

	if err := d.runtime.RemoveDriver(img); err != nil {
		return err
	}
	if err := d.removePool(language); err != nil {
		return err
	}

	logrus.Infof("driver %s removed %q", language, img.Name())
	return err
}

func (d *Daemon) DriverPool(language string) (*DriverPool, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if dp, ok := d.pool[language]; ok {
		return dp, nil
	}

	image, err := d.getDriverImage(language)
	if err != nil {
		return nil, ErrRuntime.Wrap(err)
	}

	return d.newDriverPool(language, image)
}

func (d *Daemon) getDriverImage(language string) (runtime.DriverImage, error) {
	list, err := d.runtime.ListDrivers()
	if err != nil {
		return nil, err
	}

	for _, d := range list {
		if d.Manifest.Language == language {
			return runtime.NewDriverImage(d.Reference)
		}
	}

	return nil, ErrMissingDriver.New(language)
}

// newDriverPool, instance a new driver pool for the given language and image
// and should be called under a lock.
func (d *Daemon) newDriverPool(language string, image runtime.DriverImage) (*DriverPool, error) {
	dp := NewDriverPool(func() (Driver, error) {
		logrus.Debugf("spawning driver instance %q ...", image.Name())

		opts := getDriverInstanceOptions()
		driver, err := NewDriverInstance(d.runtime, language, image, opts)
		if err != nil {
			return nil, err
		}

		if err := driver.Start(); err != nil {
			return nil, err
		}

		logrus.Infof("new driver instance started %s (%s)", image.Name(), driver.Container.ID())
		return driver, nil
	})

	dp.Logger = logrus.WithFields(logrus.Fields{
		"language": language,
	})

	d.pool[language] = dp
	return dp, dp.Start()
}

func (d *Daemon) removePool(language string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	dp, ok := d.pool[language]
	if !ok {
		return nil
	}
	if err := dp.Stop(); err != nil && !ErrPoolClosed.Is(err) {
		return err
	}
	delete(d.pool, language)
	return nil
}

// Current returns the current list of driver pools.
func (d *Daemon) Current() map[string]*DriverPool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	out := make(map[string]*DriverPool, len(d.pool))
	for k, pool := range d.pool {
		out[k] = pool
	}

	return out
}

// Stop stops all the pools and containers.
func (d *Daemon) Stop() error {
	var err error
	for _, dp := range d.pool {
		if cerr := dp.Stop(); cerr != nil && err != nil {
			err = cerr
		}
	}

	return err
}

func getDriverInstanceOptions() *Options {
	l := logrus.StandardLogger()

	opts := &Options{}
	opts.LogLevel = l.Level.String()
	opts.LogFormat = "text"

	if _, ok := l.Formatter.(*logrus.JSONFormatter); ok {
		opts.LogFormat = "json"
	}

	return opts
}

func newResponseFromError(err error) protocol1.Response {
	return protocol1.Response{
		Status: protocol1.Fatal,
		Errors: []string{err.Error()},
	}
}
