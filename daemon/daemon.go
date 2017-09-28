package daemon

import (
	"context"
	"fmt"
	"sync"

	"github.com/bblfsh/server/runtime"
	"github.com/sirupsen/logrus"

	"gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	"gopkg.in/bblfsh/sdk.v1/uast"
	"gopkg.in/src-d/go-errors.v1"
)

const (
	defaultTransport = "docker"
)

var (
	ErrMissingDriver    = errors.NewKind("missing driver for language %s")
	ErrRuntime          = errors.NewKind("runtime failure")
	ErrAlreadyInstalled = errors.NewKind("driver already installed: %s (image reference: %s)")
)

// Daemon is a Babelfish server.
type Daemon struct {
	server.Server
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
	s := &Daemon{
		version:   version,
		runtime:   r,
		pool:      make(map[string]*DriverPool),
		Overrides: make(map[string]string),
	}

	protocol.DefaultService = s
	return s
}

func (s *Daemon) AddDriver(language string, img string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, ok := s.pool[language]; ok {
		return ErrAlreadyInstalled.New(language, img)
	}

	image, err := runtime.NewDriverImage(img)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}

	if err := s.runtime.InstallDriver(image, false); err != nil {
		return ErrRuntime.Wrap(err)
	}

	s.Logger.Infof("new driver installed: %q", image.Name())
	dp := NewDriverPool(func() (Driver, error) {
		s.Logger.Debugf("spawning driver instance %q ...", image.Name())
		d, err := NewDriverInstance(s.runtime, language, image, getDriverInstanceOptions(s.Logger))
		if err != nil {
			return nil, err
		}

		if err := d.Start(); err != nil {
			return nil, err
		}

		s.Logger.Infof("driver started %s (%s)", image.Name(), d.Container.ID())
		return d, nil
	})

	s.pool[language] = dp
	return dp.Start()
}

func (s *Daemon) DriverPool(language string) (*DriverPool, error) {
	s.mutex.RLock()
	d, ok := s.pool[language]
	s.mutex.RUnlock()

	if ok {
		return d, nil
	}

	i := s.defaultDriverImageReference(language)
	err := s.AddDriver(language, i)
	if err != nil && !ErrAlreadyInstalled.Is(err) {
		return nil, ErrMissingDriver.Wrap(err, language)
	}

	s.mutex.RLock()
	d = s.pool[language]
	s.mutex.RUnlock()

	return d, nil
}

func (s *Daemon) Parse(req *protocol.ParseRequest) *protocol.ParseResponse {
	resp := &protocol.ParseResponse{}
	if req.Language == "" {
		req.Language = GetLanguage(req.Filename, []byte(req.Content))
		s.Logger.Debugf("detect language %q", req.Language)
	}

	// If the code Content is empty, just return an empty response
	if req.Content == "" {
		s.Logger.Debugf("empty code received, returning empty UAST")
		resp.Status = protocol.Ok
		resp.UAST = &uast.Node{}
		return resp
	}

	d, err := s.DriverPool(req.Language)
	if err != nil {
		resp.Status = protocol.Fatal
		resp.Errors = append(resp.Errors, "error getting driver: "+err.Error())
		return resp
	}

	err = d.Execute(func(d Driver) error {
		var err error
		resp, err = d.Service().Parse(context.Background(), req)
		return err
	})

	if err != nil {
		resp := &protocol.ParseResponse{}
		resp.Status = protocol.Fatal
		resp.Errors = append(resp.Errors, "error getting driver: "+err.Error())
	}

	return resp
}

func (s *Daemon) NativeParse(req *protocol.NativeParseRequest) *protocol.NativeParseResponse {
	return nil
}

func (s *Daemon) Version(req *protocol.VersionRequest) *protocol.VersionResponse {
	return &protocol.VersionResponse{Version: s.version}
}

func (s *Daemon) Stop() error {
	var err error
	for _, d := range s.pool {
		if cerr := d.Stop(); cerr != nil && err != nil {
			err = cerr
		}
	}

	return err
}

// returns the default image reference for a driver given a language.
func (s *Daemon) defaultDriverImageReference(lang string) string {
	if override := s.Overrides[lang]; override != "" {
		return override
	}

	transport := s.Transport
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

func getDriverInstanceOptions(logger server.Logger) *Options {
	opts := &Options{}

	var l *logrus.Logger
	switch i := logger.(type) {
	case *logrus.Logger:
		l = i
	case *logrus.Entry:
		l = i.Logger
	default:
		return opts
	}

	opts.LogLevel = l.Level.String()
	opts.LogFormat = "text"

	if _, ok := l.Formatter.(*logrus.JSONFormatter); ok {
		opts.LogFormat = "json"
	}

	return opts
}
