package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/bblfsh/sdk.v0/manifest"
)

func TestStorageInstall(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-install")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", nil}

	s := newStorage(dir)
	err = s.Install(d, false)
	require.NoError(err)
}

func TestStorageStatus(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-install")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(dir)
	err = s.Install(d, false)
	require.NoError(err)

	status, err := s.Status(d)
	require.NoError(err)
	require.False(status.Digest.IsZero())
	require.Equal("Go", status.Manifest.Language)
	require.Equal("foo", status.Reference)
}

func TestStorageStatusAndDirty(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-status")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(dir)
	err = s.Install(d, false)
	require.NoError(err)

	err = os.MkdirAll(filepath.Join(dir, "foo", ComputeDigest("bar").String()), 0777)
	require.NoError(err)
	di, err := s.Status(d)
	require.Equal(ErrDirtyDriverStorage, err)
	require.Nil(di)
}

func TestStorageStatusNotInstalled(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-status-empty")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	require.NoError(err)

	s := newStorage(dir)
	di, err := s.Status(d)
	require.Equal(ErrDriverNotInstalled, err)
	require.Nil(di)
}

func TestStorageRemove(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-remove")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", nil}

	s := newStorage(dir)

	err = s.Install(d, false)
	require.NoError(err)

	err = s.Remove(d)
	require.NoError(err)

	status, err := s.Status(d)
	require.Equal(ErrDriverNotInstalled, err)
	require.Nil(status)
}

func TestStorageRemoveEmpty(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-remove-empty")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	require.NoError(err)

	s := newStorage(dir)
	err = s.Remove(d)
	require.Equal(ErrDriverNotInstalled, err)
}

func TestStorageList(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-list")
	require.NoError(err)
	defer os.RemoveAll(dir)

	s := newStorage(dir)

	err = s.Install(&FixtureDriverImage{"//foo", nil}, false)
	require.NoError(err)
	err = s.Install(&FixtureDriverImage{"//bar", nil}, false)
	require.NoError(err)

	list, err := s.List()
	require.NoError(err)
	require.Len(list, 2)

	for _, status := range list {
		require.False(status.Digest.IsZero())
		require.True(len(status.Reference) > 0)
	}
}
