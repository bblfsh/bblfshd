//go:generate proteus -f $GOPATH/src/ -p github.com/bblfsh/bblfshd/v2/daemon/protocol --verbose
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

//proteus:generate
type DriverImageState struct {
	// Referene is the image reference from where retrieved.
	Reference string `json:"reference"`
	// This fields are from manifest.Manifest, due to some limitation of
	// proteus, can't be used directly.
	// Language of the driver.
	Language string `json:"language"`
	// Version of the driver.
	Version string `json:"version,omitempty"`
	// Build time at the compilation of the image.
	Build time.Time `json:"build,omitempty"`
	// Status is the development status of the driver (alpha, beta, etc)
	Status string `json:"status"`
	// OS is the linux distribution running on the driver container.
	//
	// Deprecated: see GoVersion and NativeVersion
	OS string `json:"os"`
	// Native version is the version of the compiler/interpreter being use in the
	// native side of the driver.
	NativeVersion []string `json:"native_version"`
	// Go version of the go runtime being use in the driver.
	GoVersion string `json:"go_version"`
}
