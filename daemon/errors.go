package daemon

import (
	"github.com/bblfsh/bblfshd/daemon/protocol"
	"gopkg.in/src-d/go-errors.v1"
)

var (
	ErrUnexpected       = errors.NewKind("unexpected error")
	ErrMissingDriver    = errors.NewKind("missing driver for language %q")
	ErrRuntime          = errors.NewKind("runtime failure")
	ErrAlreadyInstalled = protocol.ErrAlreadyInstalled

	// ErrUnauthorized indicates that image registry access failed
	// and it either requires authentication or does not exist.
	ErrUnauthorized      = errors.NewKind("unauthorized: authentication required to access %s (image: %s)")
	ErrLanguageDetection = errors.NewKind("could not autodetect language")
)
