package runtime

import (
	"fmt"
	"strings"

	"github.com/containers/image/transports"
	"github.com/containers/image/types"
	"gopkg.in/src-d/go-errors.v1"
)

var ErrInvalidImageName = errors.NewKind("invalid image name %q: %s")

// ParseImageName converts a URL-like image name to a types.ImageReference.
func ParseImageName(imgName string) (types.ImageReference, error) {
	// Copied from github.com/containers/image/transports/alltransports.go
	parts := strings.SplitN(imgName, ":", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidImageName.New(imgName, "expected colon-separated transport:reference")
	}

	transport := transports.Get(parts[0])
	if transport == nil {
		return nil, ErrInvalidImageName.New(imgName, fmt.Sprintf("unknown transport %q", parts[0]))
	}

	return transport.ParseReference(parts[1])
}
