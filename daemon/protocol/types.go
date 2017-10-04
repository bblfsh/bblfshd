//go:generate proteus -f $GOPATH/src -p github.com/bblfsh/server/daemon/protocol
//go:generate stringer -type=Status -output stringer.go

package protocol

import (
	"time"
)

// Status is the status of a driver instance.
//proteus:generate
type Status int

const (
	// Created the container exists but has not been run yet.
	Created Status = iota
	// Running the container exists and is running.
	Running
	// Pausing the container exists, it is in the process of being paused.
	Pausing
	// Paused the container exists, but all its processes are paused.
	Paused
	// Stopped the container does not have a created or running process.
	Stopped
)

//proteus:generate
type DriverPoolState struct {
	// Instances number of driver instances wanted.
	Wanted int `json:"wanted"`
	// Running number of driver instances running.
	Running int `json:"running"`
	// Waiting number of request waiting for a request be executed.
	Waiting int `json:"waiting"`
	// Success number of requests executed successfully.
	Success int `json:"success"`
	// Errors number of errors trying to process a request.
	Errors int `json:"errors"`
	// Exited number of drivers exited unexpectedly.
	Exited int `json:"exited"`
}

//proteus:generate
type DriverInstanceState struct {
	// ID of the container executing the driver.
	ID string `json:"id"`
	// Image used by the container.
	Image string
	// Status current status of the driver.
	Status Status `json:"status"`
	// Create when the driver instances was created.
	Created time.Time `json:"created"`
	// Processes are the pids of the processes running inside of the container.
	Processes []int `json:"processes"`
}
