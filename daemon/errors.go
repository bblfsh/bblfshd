package daemon

import (
	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"github.com/bblfsh/sdk/v3/driver"
	"gopkg.in/src-d/go-errors.v1"
)

var (
	// ErrUnexpected indicates unexpexted unrecoverable error condition.
	ErrUnexpected = errors.NewKind("unexpected error")
	// ErrRuntime indicates unrecoverable error condition at runtime.
	ErrRuntime = errors.NewKind("runtime failure")
	// ErrAlreadyInstalled indicates that a driver image was already installed
	// from the reference for the given language.
	ErrAlreadyInstalled = protocol.ErrAlreadyInstalled
	// ErrUnauthorized indicates that image registry access failed
	// and it either requires authentication or does not exist.
	ErrUnauthorized = errors.NewKind("unauthorized: authentication required to access %s (image: %s)")
	// ErrLanguageDetection indicates that language was not detected by Enry.
	ErrLanguageDetection = driver.ErrLanguageDetection
	// ErrUnknownEncoding is returned for parse requests with a file content in a non-UTF8 encoding.
	ErrUnknownEncoding = driver.ErrUnknownEncoding
)

// ErrMissingDriver indicates that a driver image for the given language
// can not be found.
type ErrMissingDriver = driver.ErrMissingDriver
