package core

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteTo(t *testing.T) {
	dir, err := ioutil.TempDir("", "core-driver-writeto")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)
	assert.Equal(t, d.Name(), "//busybox:latest")

	err = d.WriteTo(dir)
	assert.Nil(t, err)

	_, err = os.Stat(filepath.Join(dir, "bin/busybox"))
	assert.Nil(t, err)
}
