package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDriverImageName(t *testing.T) {
	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	assert.Equal(t, d.Name(), "busybox:latest")
}

func TestDriverImageFromNonNormalizedName(t *testing.T) {
	d, err := NewDriverImage("busybox:latest")
	assert.Nil(t, err)

	assert.Equal(t, d.Name(), "busybox:latest")
}

func TestDriverImageDigest(t *testing.T) {
	IfNetworking(t)

	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	h, err := d.Digest()
	assert.Nil(t, err)
	assert.Equal(t, h.String(), "eb34fc51f339f349df4258f68bb4a9cdbc38e3e4217cf1193a56dd0ece7d6331")
}

func TestDriverImageInspect(t *testing.T) {
	IfNetworking(t)

	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	i, err := d.Inspect()
	assert.Nil(t, err)

	assert.Equal(t, i.Os, "linux")
}

func TestDriverImageWriteTo(t *testing.T) {
	IfNetworking(t)

	dir, err := ioutil.TempDir("", "core-driver-writeto")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	err = d.WriteTo(dir)
	assert.Nil(t, err)

	_, err = os.Stat(filepath.Join(dir, "bin/busybox"))
	assert.Nil(t, err)
}
