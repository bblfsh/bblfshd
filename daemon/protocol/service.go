package protocol

import (
	"strings"
	"time"

	xcontext "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/src-d/go-errors.v1"

	"gopkg.in/bblfsh/sdk.v1/protocol"
)

var (
	ErrAlreadyInstalled = errors.NewKind("driver already installed: %s (image reference: %s)")
)

type Service interface {
	InstallDriver(language string, image string, update bool) error
	RemoveDriver(language string) error
	DriverStates() ([]*DriverImageState, error)
	DriverPoolStates() map[string]*DriverPoolState
	DriverInstanceStates() ([]*DriverInstanceState, error)
}

func RegisterService(srv *grpc.Server, s Service) {
	RegisterProtocolServiceServer(srv, &protocolServiceServer{s})
}

type protocolServiceServer struct {
	s Service
}

type Response protocol.Response

type DriverInstanceStatesResponse struct {
	protocol.Response
	// State represent the state of each driver instance in the daemon.
	State []*DriverInstanceState
}

func (s *protocolServiceServer) DriverInstanceStates(ctx xcontext.Context, _ *DriverInstanceStatesRequest) (*DriverInstanceStatesResponse, error) {
	resp := &DriverInstanceStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	var err error
	resp.State, err = s.s.DriverInstanceStates()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

type DriverPoolStatesResponse struct {
	protocol.Response
	// State represent the state of each pool in the daemon.
	State map[string]*DriverPoolState
}

func (s *protocolServiceServer) DriverPoolStates(ctx xcontext.Context, _ *DriverPoolStatesRequest) (*DriverPoolStatesResponse, error) {
	resp := &DriverPoolStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	resp.State = s.s.DriverPoolStates()
	return resp, nil
}

type DriverStatesResponse struct {
	protocol.Response
	// State represent the state of each driver in the storage.
	State []*DriverImageState
}

func (s *protocolServiceServer) DriverStates(ctx xcontext.Context, in *DriverStatesRequest) (*DriverStatesResponse, error) {
	resp := &DriverStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	var err error
	resp.State, err = s.s.DriverStates()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type InstallDriverRequest struct {
	// Language supported by the driver being installed.
	Language string
	// ImageReference is the name of the image to be installed in the following
	// format: `transport:[//]name[:tag]`. The default value for tag is `latest`
	ImageReference string
	// Update indicates whether an image should be updated. When set to false,
	// the installation fails if the image already exists.
	Update bool
}

func (s *protocolServiceServer) InstallDriver(ctx xcontext.Context, req *InstallDriverRequest) (*Response, error) {
	resp := &Response{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	err := s.s.InstallDriver(
		strings.ToLower(req.Language),
		req.ImageReference,
		req.Update,
	)

	if ErrAlreadyInstalled.Is(err) {
		return nil, status.New(codes.AlreadyExists, err.Error()).Err()
	} else if err != nil {
		return nil, err
	}
	return resp, nil
}

type RemoveDriverRequest struct {
	// Language supported by the driver to be deleted.
	Language string
}

func (s *protocolServiceServer) RemoveDriver(ctx xcontext.Context, req *RemoveDriverRequest) (result *Response, err error) {
	resp := &Response{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	if err := s.s.RemoveDriver(strings.ToLower(req.Language)); err != nil {
		return nil, err
	}
	return resp, nil
}
