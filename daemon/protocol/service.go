package protocol

import (
	"time"

	"gopkg.in/bblfsh/sdk.v1/protocol"
)

// DefaultService is the default service used to process requests.
var DefaultService Service

type Service interface {
	DriverPoolStates() map[string]*DriverPoolState
	DriverInstanceStates() ([]*DriverInstanceState, error)
	DriverStates() ([]*DriverImageState, error)
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
