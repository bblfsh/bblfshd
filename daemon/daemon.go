package daemon

import (
	"fmt"
	"sync"

	"github.com/bblfsh/server/daemon/protocol"
	"github.com/bblfsh/server/runtime"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	sdk "gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	"gopkg.in/src-d/go-errors.v1"
)

const (
	defaultTransport = "docker"
)

var (
	ErrUnexpected       = errors.NewKind("unexpected error")
	ErrMissingDriver    = errors.NewKind("missing driver for language %s")
	ErrRuntime          = errors.NewKind("runtime failure")
	ErrAlreadyInstalled = errors.NewKind("driver already installed: %s (image reference: %s)")
)

// Daemon is a Babelfish server.
type Daemon struct {
	server.Server
	ControlServer *grpc.Server

	// Transport to use to fetch driver images. Defaults to "docker".
	// Useful transports:
	// - docker: uses Docker registries (docker.io by default).
	// - docker-daemon: gets images from a local Docker daemon.
	Transport string
	// Overrides for images per language
	Overrides map[string]string

	version string
	runtime *runtime.Runtime
	mutex   sync.RWMutex
	pool    map[string]*DriverPool
}

// NewDaemon creates a new server based on the runtime with the given version.
func NewDaemon(version string, r *runtime.Runtime) *Daemon {
	d := &Daemon{
		version:       version,
		runtime:       r,
		pool:          make(map[string]*DriverPool),
		Overrides:     make(map[string]string),
		ControlServer: grpc.NewServer(),
	}

	registerGRPC(d)
	return d
}

func registerGRPC(d *Daemon) {
	sdk.DefaultService = NewService(d)

	protocol.DefaultService = NewControlService(d)
	protocol.RegisterProtocolServiceServer(
		d.ControlServer,
		protocol.NewProtocolServiceServer(),
	)
}

func (d *Daemon) AddDriver(language string, img string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if _, ok := d.pool[language]; ok {
		return ErrAlreadyInstalled.New(language, img)
	}

	image, err := runtime.NewDriverImage(img)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}

	if err := d.runtime.InstallDriver(image, false); err != nil {
		return ErrRuntime.Wrap(err)
	}

	logrus.Infof("new driver installed: %q", image.Name())
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
	return dp.Start()
}

func (d *Daemon) DriverPool(language string) (*DriverPool, error) {
	d.mutex.RLock()
	dp, ok := d.pool[language]
	d.mutex.RUnlock()

	if ok {
		return dp, nil
	}

	i := d.defaultDriverImageReference(language)
	err := d.AddDriver(language, i)
	if err != nil && !ErrAlreadyInstalled.Is(err) {
		return nil, ErrMissingDriver.Wrap(err, language)
	}

	d.mutex.RLock()
	dp = d.pool[language]
	d.mutex.RUnlock()

	return dp, nil
}

// Current returns the current list of driver pools.
func (d *Daemon) Current() map[string]*DriverPool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

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

// returns the default image reference for a driver given a language.
func (d *Daemon) defaultDriverImageReference(lang string) string {
	if override := d.Overrides[lang]; override != "" {
		return override
	}

	transport := d.Transport
	if transport == "" {
		transport = defaultTransport
	}

	ref := fmt.Sprintf("bblfsh/%s-driver:latest", lang)
	switch transport {
	case "docker":
		ref = "//" + ref
	}

	return fmt.Sprintf("%s:%s", transport, ref)
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

func newResponseFromError(err error) sdk.Response {
	return sdk.Response{
		Status: sdk.Fatal,
		Errors: []string{err.Error()},
	}
}
