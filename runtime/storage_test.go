package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/bblfsh/sdk.v1/manifest"
)

func TestStorageInstall(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-install")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	m, err := s.Install(d, false)
	require.NoError(err)
	require.NotNil(m)
	require.Equal(m.Manifest.Language, "Go")
}

func TestStorageStatus(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-install")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	_, err = s.Install(d, false)
	require.NoError(err)

	status, err := s.Status(d)
	require.NoError(err)
	require.False(status.Digest.IsZero())
	require.Equal("Go", status.Manifest.Language)
	require.Equal("//foo", status.Reference)
}

func TestStorageStatus_Dirty(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-status")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{Language: "Go"}}

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	_, err = s.Install(d, false)
	require.NoError(err)

	err = os.MkdirAll(filepath.Join(dir, "images",
		ComputeDigest("//foo").String(),
		ComputeDigest("bar").String(),
	), 0777)
	require.NoError(err)

	di, err := s.Status(d)
	require.True(ErrDirtyDriverStorage.Is(err))
	require.Nil(di)
}

func TestStorageStatus_NotInstalled(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-status-empty")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	require.NoError(err)

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	di, err := s.Status(d)
	require.True(ErrDriverNotInstalled.Is(err))
	require.Nil(di)
}

func TestStorageRemove(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-remove")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d := &FixtureDriverImage{"//foo", &manifest.Manifest{}}

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	_, err = s.Install(d, false)
	require.NoError(err)

	err = s.Remove(d)
	require.NoError(err)

	status, err := s.Status(d)
	require.True(ErrDriverNotInstalled.Is(err))
	require.Nil(status)
}

func TestStorageRemove_Empty(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-remove-empty")
	require.NoError(err)
	defer os.RemoveAll(dir)

	d, err := NewDriverImage("docker://busybox:latest")
	require.NoError(err)

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))
	err = s.Remove(d)
	require.True(ErrDriverNotInstalled.Is(err))
}

func TestStorageList(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "runtime-storage-list")
	require.NoError(err)
	defer os.RemoveAll(dir)

	s := newStorage(filepath.Join(dir, "images"), filepath.Join(dir, "tmp"))

	_, err = s.Install(&FixtureDriverImage{"//foo", &manifest.Manifest{}}, false)
	require.NoError(err)

	_, err = s.Install(&FixtureDriverImage{"//bar/bar", &manifest.Manifest{}}, false)
	require.NoError(err)

	list, err := s.List()
	require.NoError(err)
	require.Len(list, 2)

	for _, status := range list {
		require.False(status.Digest.IsZero())
		require.True(len(status.Reference) > 0)
	}
}
