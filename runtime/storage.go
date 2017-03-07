package runtime

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/bblfsh/sdk/manifest"
)

var (
	ErrDirtyDriverStorage = errors.New("dirty driver storage")
	ErrDriverNotInstalled = errors.New("driver not installed")
)

// storage represents the DriverImage storage, taking care of filesystem
// image operations, such as install, update, remove, etc.
type storage struct {
	path string
}

func newStorage(path string) *storage {
	return &storage{path: path}
}

// Install installs a DriverImage extracting his content to the filesystem,
// only one version per image can be stored, update is required to overwrite a
// previous image if already exists otherwise, Install fails if an previous
// image already exists.
func (s *storage) Install(d DriverImage, update bool) error {
	current, err := s.RootFS(d)
	if err != nil && err != ErrDriverNotInstalled {
		return err
	}

	exists := current != ""
	if exists && !update {
		return nil
	}

	di, err := d.Digest()
	if err != nil {
		return err
	}

	if exists {
		if err := s.Remove(d); err != nil {
			return err
		}
	}

	rootfs := s.rootFSPath(d, di)
	return d.WriteTo(rootfs)
}

// RootFS returns the path in the host filesystem to an installed image.
func (s *storage) RootFS(d DriverImage) (string, error) {
	return s.rootFSFromBase(s.basePath(d))
}

func (s *storage) rootFSFromBase(path string) (string, error) {
	dirs, err := getDirs(path)
	if err != nil {
		return "", err
	}

	switch len(dirs) {
	case 1:
		return dirs[0], nil
	case 0:
		return "", ErrDriverNotInstalled
	default:
		return "", ErrDirtyDriverStorage
	}
}

// Status returns the current status in the storage for a given DriverImage, nil
// is returned if the image is not installed.
func (s *storage) Status(d DriverImage) (*DriverImageStatus, error) {
	path, err := s.RootFS(d)
	if err != nil {
		return nil, err
	}

	return newDriverImageStatus(path)
}

// Remove removes a given DriverImage from the filesystem.
func (s *storage) Remove(d DriverImage) error {
	path, err := s.RootFS(d)
	if err != nil {
		return err
	}

	return os.RemoveAll(path)
}

// List lists all the driver images installed on disk.
func (s *storage) List() ([]*DriverImageStatus, error) {
	dirs, err := getDirs(s.path)
	if err != nil {
		return nil, err
	}

	var list []*DriverImageStatus
	for _, base := range dirs {
		root, err := s.rootFSFromBase(base)
		if err != nil {
			return nil, err
		}

		status, err := newDriverImageStatus(root)
		if err != nil {
			return nil, err
		}

		list = append(list, status)
	}

	return list, nil
}

func (s *storage) rootFSPath(d DriverImage, di Digest) string {
	return filepath.Join(s.basePath(d), di.String())
}

func (s *storage) basePath(d DriverImage) string {
	return filepath.Join(s.path, d.Name())
}

func (s *storage) basePathExists(d DriverImage) (bool, error) {
	path := s.basePath(d)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func getDirs(path string) ([]string, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	var dirs []string
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		dirs = append(dirs, filepath.Join(path, f.Name()))
	}

	return dirs, nil
}

func newDriverImageStatus(path string) (*DriverImageStatus, error) {
	manifest, err := manifest.Load(filepath.Join(path, manifest.Filename))
	if err != nil {
		return nil, err
	}

	base, digest := filepath.Split(path)
	name := filepath.Base(base)

	return &DriverImageStatus{
		Reference: name,
		Digest:    NewDigest(digest),
		Manifest:  manifest,
	}, nil
}

// DriverImageStatus represents the status of an installed driver image on disk.
type DriverImageStatus struct {
	Reference string
	Digest    Digest
	Manifest  *manifest.Manifest
}
