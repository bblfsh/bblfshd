package protocol

import (
	"strings"
	"time"

	"gopkg.in/bblfsh/sdk.v1/protocol"
)

// DefaultService is the default service used to process requests.
var DefaultService Service

type Service interface {
	InstallDriver(language string, image string, update bool) error
	RemoveDriver(language string) error
	DriverStates() ([]*DriverImageState, error)
	DriverPoolStates() map[string]*DriverPoolState
	DriverInstanceStates() ([]*DriverInstanceState, error)
}

//proteus:generate
type DriverPoolStatesResponse struct {
	protocol.Response
	// State represent the state of each pool in the daemon.
	State map[string]*DriverPoolState
}

//proteus:generate
func DriverPoolStates() *DriverPoolStatesResponse {
	resp := &DriverPoolStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	resp.State = DefaultService.DriverPoolStates()
	return resp
}

//proteus:generate
type DriverInstanceStatesResponse struct {
	protocol.Response
	// State represent the state of each driver instance in the daemon.
	State []*DriverInstanceState
}

//proteus:generate
func DriverInstanceStates() *DriverInstanceStatesResponse {
	resp := &DriverInstanceStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	var err error
	resp.State, err = DefaultService.DriverInstanceStates()
	if err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	return resp
}

//proteus:generate
type DriverStatesResponse struct {
	protocol.Response
	// State represent the state of each driver in the storage.
	State []*DriverImageState
}

//proteus:generate
func DriverStates() *DriverStatesResponse {
	resp := &DriverStatesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	var err error
	resp.State, err = DefaultService.DriverStates()
	if err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	return resp
}

//proteus:generate
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

type Response protocol.Response

//proteus:generate
func InstallDriver(req *InstallDriverRequest) *Response {
	resp := &Response{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	err := DefaultService.InstallDriver(
		strings.ToLower(req.Language),
		req.ImageReference,
		req.Update,
	)

	if err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	return resp
}

//proteus:generate
type RemoveDriverRequest struct {
	// Language supported by the driver to be deleted.
	Language string
}

//proteus:generate
func RemoveDriver(req *RemoveDriverRequest) *Response {
	resp := &Response{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
	}()

	if err := DefaultService.RemoveDriver(strings.ToLower(req.Language)); err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	return resp
}
