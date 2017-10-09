package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/types"
	"gopkg.in/bblfsh/sdk.v1/manifest"
	"gopkg.in/bblfsh/sdk.v1/sdk/driver"
)

const FixtureReference = "docker-daemon:bblfsh/bblfshd:fixture"

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

	if err := WriteImageConfig(&ImageConfig{ImageRef: d.N}, path); err != nil {
		return err
	}

	if d.M == nil {
		return nil
	}

	m := filepath.Join(path, driver.ManifestLocation)
	if err := os.MkdirAll(filepath.Dir(m), 0777); err != nil {
		return err
	}

	w, err := os.Create(m)
	if err != nil {
		return err
	}

	defer w.Close()
	return d.M.Encode(w)
}
