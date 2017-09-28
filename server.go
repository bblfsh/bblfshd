package server

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

// Server is a Babelfish server.
type Server struct {
	server.Server
	// Transport to use to fetch driver images. Defaults to "docker".
	// Useful transports:
	// - docker: uses Docker registries (docker.io by default).
	// - docker-daemon: gets images from a local Docker daemon.
	Transport string
	// Overrides for images per language
	Overrides map[string]string

	version string
	rt      *runtime.Runtime
	mu      sync.RWMutex
	pool    map[string]*DriverPool
}

func NewServer(version string, r *runtime.Runtime) *Server {
	s := &Server{
		rt:        r,
		pool:      make(map[string]*DriverPool),
		Overrides: make(map[string]string),
	}

	protocol.DefaultService = s
	return s
}

func (s *Server) AddDriver(lang string, img string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pool[lang]
	if ok {
		return ErrAlreadyInstalled.New(lang, img)
	}

	image, err := runtime.NewDriverImage(img)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}

	if err := s.rt.InstallDriver(image, false); err != nil {
		return ErrRuntime.Wrap(err)
	}

	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		d, err := NewDriverInstance(s.rt, image, &Options{Verbosity: "debug"})
		if err != nil {
			return nil, err
		}

		return d, d.Start()
	})
	if err != nil {
		return err
	}

	s.pool[lang] = dp
	return nil
}

func (s *Server) DriverPool(lang string) (*DriverPool, error) {
	s.mu.RLock()
	d, ok := s.pool[lang]
	s.mu.RUnlock()
	if !ok {
		img := s.defaultDriverImageReference(lang)
		err := s.AddDriver(lang, img)
		if err != nil && !ErrAlreadyInstalled.Is(err) {
			return nil, ErrMissingDriver.Wrap(err, lang)
		}

		s.mu.RLock()
		d = s.pool[lang]
		s.mu.RUnlock()
	}

	return d, nil
}

func (s *Server) Parse(req *protocol.ParseRequest) *protocol.ParseResponse {
	resp := &protocol.ParseResponse{}

	if req.Language == "" {
		req.Language = GetLanguage(req.Filename, []byte(req.Content))
		logrus.Debug("detect language %q", req.Language)
	}

	// If the code Content is empty, just return an empty reponse
	if req.Content == "" {
		logrus.Debug("empty code received, returning empty UAST")
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

	d.Execute(func(d Driver) error {
		var err error
		resp, err = d.Service().Parse(context.Background(), req)
		return err
	})

	return resp
}

func (s *Server) NativeParse(req *protocol.NativeParseRequest) *protocol.NativeParseResponse {
	return nil
}

func (s *Server) Version(req *protocol.VersionRequest) *protocol.VersionResponse {
	return &protocol.VersionResponse{Version: s.version}
}

func (s *Server) Stop() error {
	var err error
	for _, d := range s.pool {
		if cerr := d.Stop(); cerr != nil && err != nil {
			err = cerr
		}
	}

	return err
}

// returns the default image reference for a driver given a language.
func (s *Server) defaultDriverImageReference(lang string) string {
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
