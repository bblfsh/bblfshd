package runtime 

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstall(t *testing.T) {
	IfNetworking(t)

	dir, err := ioutil.TempDir("", "core-storage-install")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)

	s := NewStorage(dir)
	err = s.Install(d, false)
	assert.Nil(t, err)
}

func TestStatusAndDirty(t *testing.T) {
	dir, err := ioutil.TempDir("", "core-storage-status")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)

	expected := ComputeDigest("foo")
	err = os.MkdirAll(filepath.Join(dir, "busybox:latest", expected.String()), 0777)
	assert.Nil(t, err)

	s := NewStorage(dir)
	di, err := s.Status(d)
	assert.Nil(t, err)
	assert.Equal(t, expected, di)

	err = os.MkdirAll(filepath.Join(dir, "busybox:latest", ComputeDigest("bar").String()), 0777)
	assert.Nil(t, err)
	di, err = s.Status(d)
	assert.Equal(t, ErrDirtyDriverStorage, err)
	assert.True(t, di.IsZero())
}

func TestStatusEmpty(t *testing.T) {
	dir, err := ioutil.TempDir("", "core-storage-status-empty")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)

	s := NewStorage(dir)
	di, err := s.Status(d)
	assert.Nil(t, err)
	assert.True(t, di.IsZero())
}

func TestRemove(t *testing.T) {
	dir, err := ioutil.TempDir("", "core-storage-remove")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)

	err = os.MkdirAll(filepath.Join(dir, "busybox:latest", ComputeDigest("bar").String()), 0777)
	assert.Nil(t, err)

	s := NewStorage(dir)
	err = s.Remove(d)
	assert.Nil(t, err)

	dirs, err := getDirs(filepath.Join(dir, "busybox:latest"))
	assert.Nil(t, err)
	assert.Len(t, dirs, 0)
}

func TestRemoveEmpty(t *testing.T) {
	dir, err := ioutil.TempDir("", "core-storage-remove-empty")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	assert.Nil(t, err)

	s := NewStorage(dir)
	err = s.Remove(d)
	assert.Nil(t, err)
}
