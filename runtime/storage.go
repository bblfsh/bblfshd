package runtime

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
)

var ErrDirtyDriverStorage = errors.New("dirty driver storage")

type storage struct {
	path string
}

func NewStorage(path string) *storage {
	return &storage{path: path}
}

func (s *storage) Install(d DriverImage, update bool) error {
	current, err := s.Status(d)
	if err != nil {
		return err
	}

	exists := !current.IsZero()
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

func (s *storage) RootFS(d DriverImage) (string, error) {
	current, err := s.Status(d)
	if err != nil {
		return "", err
	}

	return s.rootFSPath(d, current), nil
}

func (s *storage) Status(d DriverImage) (Digest, error) {
	dirs, err := getDirs(s.basePath(d))
	if err != nil {
		return nil, err
	}

	switch len(dirs) {
	case 1:
		return NewDigest(dirs[0]), nil
	case 0:
		return nil, nil
	default:
		return nil, ErrDirtyDriverStorage
	}
}

func (s *storage) Remove(d DriverImage) error {
	path, err := s.RootFS(d)
	if err != nil {
		return err
	}

	return os.RemoveAll(path)
}

func (s *storage) rootFSPath(d DriverImage, di Digest) string {
	return filepath.Join(s.basePath(d), di.String())
}

func (s *storage) basePath(d DriverImage) string {
	return filepath.Join(s.path, d.Name()[2:])
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

		dirs = append(dirs, f.Name())
	}

	return dirs, nil
}
