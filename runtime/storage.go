package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/bblfsh/sdk.v1/manifest"
	"gopkg.in/bblfsh/sdk.v1/sdk/driver"
	"gopkg.in/src-d/go-errors.v1"
)

var (
	ErrDirtyDriverStorage = errors.NewKind("dirty driver storage")
	ErrDriverNotInstalled = errors.NewKind("driver not installed")
	ErrMalformedDriver    = errors.NewKind("malformed driver, missing manifest.toml")
)

// storage represents the DriverImage storage, taking care of filesystem
// image operations, such as install, update, remove, etc.
type storage struct {
	path string
	temp string
}

func newStorage(path, temp string) *storage {
	return &storage{path: path, temp: temp}
}

// Install installs a DriverImage extracting his content to the filesystem,
// only one version per image can be stored, update is required to overwrite a
// previous image if already exists otherwise, Install fails if an previous
// image already exists.
func (s *storage) Install(d DriverImage, update bool) (*DriverImageStatus, error) {
	current, err := s.RootFS(d)
	if err != nil && !ErrDriverNotInstalled.Is(err) {
		return nil, err
	}

	exists := current != ""
	if exists && !update {
		return nil, nil
	}

	di, err := d.Digest()
	if err != nil {
		return nil, err
	}

	if exists {
		if err := s.Remove(d); err != nil {
			return nil, err
		}
	}

	tmp, err := s.tempPath()
	if err != nil {
		return nil, err
	}

	if err := d.WriteTo(tmp); err != nil {
		return nil, err
	}

	m, err := newDriverImageStatus(tmp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMalformedDriver.New()
		}

		return nil, err
	}
	return m, s.moveImage(tmp, d, di)
}

func (s *storage) tempPath() (string, error) {
	if err := os.MkdirAll(s.temp, 0655); err != nil {
		return "", err
	}

	return ioutil.TempDir(s.temp, "image")
}

func (s *storage) moveImage(source string, d DriverImage, di Digest) error {
	root := s.rootFSPath(d, di)
	dir := filepath.Dir(root)
	if err := os.MkdirAll(dir, 0655); err != nil {
		return err
	}

	if err := os.Rename(source+configExt, root+configExt); err != nil {
		return err
	}

	return os.Rename(source, root)
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
		return "", ErrDriverNotInstalled.New()
	default:
		return "", ErrDirtyDriverStorage.New()
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

	if err := os.RemoveAll(path + configExt); err != nil {
		return err
	}

	return os.RemoveAll(path)
}

// List lists all the driver images installed on disk.
func (s *storage) List() ([]*DriverImageStatus, error) {
	config, err := filepath.Glob(filepath.Join(s.path, "*/*"+configExt))
	if err != nil {
		return nil, err
	}

	var list []*DriverImageStatus
	for _, c := range config {
		root := c[:len(c)-len(configExt)]
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
	return filepath.Join(s.path, ComputeDigest(d.Name()).String())
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
	manifest, err := manifest.Load(filepath.Join(path, driver.ManifestLocation))
	if err != nil {
		return nil, err
	}

	config, err := ReadImageConfig(path)
	if err != nil {
		return nil, err
	}

	_, digest := filepath.Split(path)
	return &DriverImageStatus{
		Reference: config.ImageRef,
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
