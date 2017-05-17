package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriverImageName(t *testing.T) {
	require := require.New(t)

	d, err := NewDriverImage("//busybox:latest")
	require.NoError(err)
	require.Equal("busybox:latest", d.Name())
}

func TestDriverImageFromNonNormalizedName(t *testing.T) {
	require := require.New(t)

	d, err := NewDriverImage("busybox:latest")
	require.NoError(err)
	require.Equal("busybox:latest", d.Name())
}

func TestDriverImageDigest(t *testing.T) {
	require := require.New(t)
	IfNetworking(t)
	
	d, err := NewDriverImage("//smolav/busybox-test-image:latest")
	require.NoError(err)

	h, err := d.Digest()
	require.NoError(err)
	require.Equal("116d67f147f35850964c8c98231a3316623cb1de4d6fa29a12587b8882e69c4c", h.String())
}

func TestDriverImageInspect(t *testing.T) {
	require := require.New(t)
	IfNetworking(t)

	d, err := NewDriverImage("//busybox:latest")
	require.NoError(err)

	i, err := d.Inspect()
	require.NoError(err)
	require.Equal("linux", i.Os)
}

func TestDriverImageWriteTo(t *testing.T) {
	require := require.New(t)
	IfNetworking(t)

	dir, err := ioutil.TempDir("", "core-driver-writeto")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("//busybox:latest")
	require.NoError(err)

	err = d.WriteTo(dir)
	require.NoError(err)

	_, err = os.Stat(filepath.Join(dir, "bin/busybox"))
	require.NoError(err)
}
