package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bblfsh/sdk/manifest"
	"github.com/stretchr/testify/assert"
)

func TestStorageInstall(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-install")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", nil}

	s := newStorage(dir)
	err = s.Install(d, false)
	assert.Nil(t, err)
}

func TestStorageStatus(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-install")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(dir)
	err = s.Install(d, false)
	assert.Nil(t, err)

	status, err := s.Status(d)
	assert.Nil(t, err)
	assert.False(t, status.Digest.IsZero())
	assert.Equal(t, "Go", status.Manifest.Language)
	assert.Equal(t, "foo", status.Reference)
}

func TestStorageStatusAndDirty(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-status")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(dir)
	err = s.Install(d, false)
	assert.Nil(t, err)

	err = os.MkdirAll(filepath.Join(dir, "foo", ComputeDigest("bar").String()), 0777)
	assert.Nil(t, err)
	di, err := s.Status(d)
	assert.Equal(t, ErrDirtyDriverStorage, err)
	assert.Nil(t, di)
}

func TestStorageStatusNotInstalled(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-status-empty")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	s := newStorage(dir)
	di, err := s.Status(d)
	assert.Equal(t, ErrDriverNotInstalled, err)
	assert.Nil(t, di)
}

func TestStorageRemove(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-remove")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", nil}

	s := newStorage(dir)

	err = s.Install(d, false)
	assert.Nil(t, err)

	err = s.Remove(d)
	assert.Nil(t, err)

	status, err := s.Status(d)
	assert.Equal(t, ErrDriverNotInstalled, err)
	assert.Nil(t, status)
}

func TestStorageRemoveEmpty(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-remove-empty")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("//busybox:latest")
	assert.Nil(t, err)

	s := newStorage(dir)
	err = s.Remove(d)
	assert.Equal(t, ErrDriverNotInstalled, err)
}

func TestStorageList(t *testing.T) {
	dir, err := ioutil.TempDir("", "runtime-storage-list")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	s := newStorage(dir)

	err = s.Install(&FixtureDriverImage{"//foo", nil}, false)
	assert.Nil(t, err)
	err = s.Install(&FixtureDriverImage{"//bar", nil}, false)
	assert.Nil(t, err)

	list, err := s.List()
	assert.Nil(t, err)
	assert.Len(t, list, 2)

	for _, status := range list {
		assert.False(t, status.Digest.IsZero())
		assert.True(t, len(status.Reference) > 0)
	}
}
