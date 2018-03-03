package runtime

import (
	_ "github.com/containers/image/directory"
	_ "github.com/containers/image/docker"
	_ "github.com/containers/image/docker/archive"
	_ "github.com/containers/image/docker/daemon"
	_ "github.com/containers/image/oci/layout"
)
