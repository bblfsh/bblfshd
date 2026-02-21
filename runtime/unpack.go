package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containers/image/pkg/blobinfocache/none"
	"github.com/containers/image/types"
	"github.com/pkg/errors"
)

func UnpackImage(src types.Image, target string) error {
	ctx := context.Background()
	ref := src.Reference()
	unpackLayer, err := getLayerUnpacker(ref)
	if err != nil {
		return err
	}

	raw, err := ref.NewImageSource(ctx, nil)
	if err != nil {
		return err
	}

	for _, layer := range src.LayerInfos() {
		rc, _, err := raw.GetBlob(ctx, layer, none.NoCache)
		if err != nil {
			return err
		}

		if err := unpackLayer(target, rc); err != nil {
			return err
		}
	}

	return nil
}

func getLayerUnpacker(ref types.ImageReference) (func(string, io.Reader) error, error) {
	transport := ref.Transport().Name()
	switch transport {
	case "docker-daemon":
		return untar, nil
	case "docker":
		return untarGzip, nil
	default:
		return nil, fmt.Errorf("unsupported transport: %s", transport)
	}
}

func untarGzip(dest string, r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrap(err, "error creating gzip reader")
	}
	defer gz.Close()

	return untar(dest, gz)
}

func untar(dest string, r io.Reader) error {
	entries := make(map[string]bool)
	var dirs []*tar.Header
	tr := tar.NewReader(r)
loop:
	for {
		hdr, err := tr.Next()
		switch err {
		case io.EOF:
			break loop
		case nil:
			// success, continue below
		default:
			return errors.Wrapf(err, "error advancing tar stream")
		}

		hdr.Name = filepath.Clean(hdr.Name)
		if !strings.HasSuffix(hdr.Name, string(os.PathSeparator)) {
			// Not the root directory, ensure that the parent directory exists
			parent := filepath.Dir(hdr.Name)
			parentPath := filepath.Join(dest, parent)
			if _, err2 := os.Lstat(parentPath); err2 != nil && os.IsNotExist(err2) {
				if err3 := os.MkdirAll(parentPath, 0755); err3 != nil {
					return fmt.Errorf("error creating parent directory: %w", err3)
				}
			}
		}
		path := filepath.Join(dest, hdr.Name)
		if entries[path] {
			return fmt.Errorf("duplicate entry for %s", path)
		}
		entries[path] = true
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		info := hdr.FileInfo()
		if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%q is outside of %q", hdr.Name, dest)
		}

		if strings.HasPrefix(info.Name(), ".wh.") {
			path = strings.Replace(path, ".wh.", "", 1)

			if err := os.RemoveAll(path); err != nil {
				return errors.Wrap(err, "unable to delete whiteout path")
			}

			continue loop
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if fi, err := os.Lstat(path); !(err == nil && fi.IsDir()) {
				if err2 := os.MkdirAll(path, info.Mode()); err2 != nil {
					return errors.Wrap(err2, "error creating directory")
				}
			}

		case tar.TypeReg, tar.TypeRegA:
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, info.Mode())
			if err != nil {
				return errors.Wrap(err, "unable to open file")
			}
			_, err = io.Copy(f, tr)
			f.Close()

			if err != nil {
				return errors.Wrap(err, "unable to copy")
			}

		case tar.TypeLink:
			// FIXME: we need to check if the target is inside dest (including traversing symlinks)
			target := filepath.Join(dest, hdr.Linkname)

			if !strings.HasPrefix(target, dest) {
				return fmt.Errorf("invalid hardlink %q -> %q", target, hdr.Linkname)
			}

			if err := os.Link(target, path); err != nil {
				return fmt.Errorf("failed to hardlink %q -> %q: %w", target, hdr.Linkname, err)
			}

		case tar.TypeSymlink:
			// FIXME: we need to check if the target is inside dest (including traversing symlinks)
			target := filepath.Join(filepath.Dir(path), hdr.Linkname)
			if filepath.IsAbs(hdr.Linkname) {
				target = filepath.Join(dest, hdr.Linkname)
			}

			if !strings.HasPrefix(target, dest) {
				return fmt.Errorf("invalid symlink %q -> %q", path, hdr.Linkname)
			}

			if err := os.Symlink(hdr.Linkname, path); err != nil {
				if os.IsExist(err) {
					if err := os.Remove(path); err != nil {
						return fmt.Errorf("cannot replace symlink %q -> %q: %w", path, hdr.Linkname, err)
					}

					if err := os.Symlink(hdr.Linkname, path); err != nil {
						return fmt.Errorf("cannot symlink %q -> %q: %w", path, hdr.Linkname, err)
					}
				}
			}

		case tar.TypeXGlobalHeader:
			return nil
		}
		// Directory mtimes must be handled at the end to avoid further
		// file creation in them to modify the directory mtime
		if hdr.Typeflag == tar.TypeDir {
			dirs = append(dirs, hdr)
		}
	}
	for _, hdr := range dirs {
		path := filepath.Join(dest, hdr.Name)

		finfo := hdr.FileInfo()
		// I believe the old version was using time.Now().UTC() to overcome an
		// invalid error from chtimes.....but here we lose hdr.AccessTime like this...
		if err := os.Chtimes(path, time.Now().UTC(), finfo.ModTime()); err != nil {
			return errors.Wrap(err, "error changing time")
		}
	}
	return nil
}
