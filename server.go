package server

import (
	"fmt"
	"net"
	"sync"

	"github.com/bblfsh/server/runtime"
	"github.com/bblfsh/sdk/uast"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"google.golang.org/grpc"
	"gopkg.in/src-d/go-errors.v0"
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
	version string
	// Transport to use to fetch driver images. Defaults to "docker".
	// Useful transports:
	// - docker: uses Docker registries (docker.io by default).
	// - docker-daemon: gets images from a local Docker daemon.
	Transport string
	rt        *runtime.Runtime
	mu        sync.RWMutex
	drivers   map[string]Driver
	overrides map[string]string // Overrides for images per language
}

func NewServer(v string, r *runtime.Runtime, overrides map[string]string) *Server {
	return &Server{
		version:   v,
		rt:        r,
		drivers:   make(map[string]Driver),
		overrides: overrides,
	}
}

func (s *Server) Serve(listener net.Listener, maxMessageSize int) error {
	opts := []grpc.ServerOption{}
	if maxMessageSize != 0 {
		logrus.Infof("setting maximum size for sending and receiving messages to %d", maxMessageSize)
		opts = append(opts, grpc.MaxRecvMsgSize(maxMessageSize))
		opts = append(opts, grpc.MaxSendMsgSize(maxMessageSize))
	}

	grpcServer := grpc.NewServer(opts...)

	logrus.Debug("registering gRPC service")
	protocol.RegisterProtocolServiceServer(
		grpcServer,
		protocol.NewProtocolServiceServer(),
	)

	protocol.DefaultParser = s
	protocol.DefaultVersioner = s

	logrus.Info("starting gRPC server")
	return grpcServer.Serve(listener)
}

func (s *Server) AddDriver(lang string, img string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.drivers[lang]
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
		return ExecDriver(s.rt, image)
	})
	if err != nil {
		return err
	}

	s.drivers[lang] = dp
	return nil
}

func (s *Server) Driver(lang string) (Driver, error) {
	s.mu.RLock()
	d, ok := s.drivers[lang]
	s.mu.RUnlock()
	if !ok {
		img := s.defaultDriverImageReference(lang)
		err := s.AddDriver(lang, img)
		if err != nil && !ErrAlreadyInstalled.Is(err) {
			return nil, ErrMissingDriver.Wrap(err, lang)
		}

		s.mu.RLock()
		d = s.drivers[lang]
		s.mu.RUnlock()
	}

	return d, nil
}

func (s *Server) Parse(req *protocol.ParseRequest) *protocol.ParseResponse {
	lang := req.Language
	if lang == "" {
		lang = GetLanguage(req.Filename, []byte(req.Content))
	}

	// If the code Content is empty, just return an empty reponse
	if req.Content == "" {
		logrus.Debug("Empty code received, returning empty UAST")
		return &protocol.ParseResponse{
			Status: protocol.Ok,
			UAST: &uast.Node{},
		}
	}

	d, err := s.Driver(lang)
	if err != nil {
		return &protocol.ParseResponse{
			Status: protocol.Fatal,
			Errors: []string{"error getting driver: " + err.Error()},
		}
	}

	return d.Parse(req)
}

func (s *Server) Version(req *protocol.VersionRequest) *protocol.VersionResponse {
	return &protocol.VersionResponse{Version: s.version}
}

func (s *Server) Close() error {
	var err error
	for _, d := range s.drivers {
		if cerr := d.Close(); cerr != nil && err != nil {
			err = cerr
		}
	}

	return err
}

// returns the default image reference for a driver given a language.
func (s *Server) defaultDriverImageReference(lang string) string {
	if override := s.overrides[lang]; override != "" {
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
