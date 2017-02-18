package runtime

import (
	"flag"
	"testing"
)

var networking = flag.Bool("network", false, "excute tests using network")

func init() {
	flag.Parse()
}

func IfNetworking(t *testing.T) {
	if *networking {
		return
	}

	t.Skip("skipping network test use --network to run this test")
}
