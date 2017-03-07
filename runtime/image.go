package runtime

import (
	"fmt"
	"strings"

	"github.com/bblfsh/server/utils"

	"github.com/containers/image/docker"
	"github.com/containers/image/image"
	"github.com/containers/image/types"
)

// DriverImage represents a docker image of a driver
type DriverImage interface {
	Name() string
	Digest() (Digest, error)
	Inspect() (*types.ImageInspectInfo, error)
	WriteTo(path string) error
}

type driverImage struct {
	ref types.ImageReference
}

// NewDriverImage returns a new DriverImage from a docker image reference.
// The format of imageRef is defined by docker.ParseReference, the format can be
// a non-normalized string like `bblfsh/rust-driver:lastest` or a normalized
// referene like `//bblfsh/rust-driver:lastest`
func NewDriverImage(imageRef string) (DriverImage, error) {
	imageRef = strings.TrimPrefix(imageRef, "//")
	ref, err := docker.ParseReference(fmt.Sprintf("//%s", imageRef))
	if err != nil {
		return nil, fmt.Errorf("invalid source ref %s: %v", imageRef, err)
	}

	return &driverImage{ref: ref}, nil
}

// Name returns the name of the driver image based on the image reference.
func (d *driverImage) Name() string {
	return strings.TrimPrefix(d.ref.StringWithinTransport(), "//")
}

// Digest computes a digest based on the image layers.
func (d *driverImage) Digest() (Digest, error) {
	img, err := d.image()
	if err != nil {
		return nil, err
	}

	defer img.Close()
	i, err := img.Inspect()
	if err != nil {
		return nil, err
	}

	return ComputeDigest(i.Layers...), nil
}

func (d *driverImage) Inspect() (*types.ImageInspectInfo, error) {
	img, err := d.image()
	if err != nil {
		return nil, err
	}

	defer img.Close()
	return img.Inspect()
}

// WriteTo writes the image to disk at the given path.
func (d *driverImage) WriteTo(path string) error {
	img, err := d.image()
	if err != nil {
		return err
	}

	defer img.Close()
	return utils.UnpackImage(img, path)
}

func (d *driverImage) image() (types.Image, error) {
	raw, err := d.ref.NewImageSource(nil, nil)
	if err != nil {
		return nil, err
	}

	unparsedImage := image.UnparsedFromSource(raw)
	return image.FromUnparsedImage(unparsedImage)
}
