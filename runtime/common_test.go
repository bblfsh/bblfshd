package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/types"
	"gopkg.in/bblfsh/sdk.v0/manifest"
)

func init() {
	Bootstrap()
}

func IfNetworking(t *testing.T) {
	if len(os.Getenv("TEST_NETWORKING")) != 0 {
		return
	}

	t.Skip("skipping network test use TEST_NETWORKING to run this test")
}

type FixtureDriverImage struct {
	N string
	M *manifest.Manifest
}

func (d *FixtureDriverImage) Name() string {
	return d.N
}

func (d *FixtureDriverImage) Digest() (Digest, error) {
	return ComputeDigest(d.N), nil
}

func (d *FixtureDriverImage) Inspect() (*types.ImageInspectInfo, error) {
	return nil, nil
}

func (d *FixtureDriverImage) WriteTo(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	w, err := os.Create(filepath.Join(path, manifest.Filename))
	if err != nil {
		return err
	}

	defer w.Close()

	if d.M == nil {
		d.M = &manifest.Manifest{}
	}

	return d.M.Encode(w)
}
