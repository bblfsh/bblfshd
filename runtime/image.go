package runtime

import (
	"strings"

	"github.com/bblfsh/server/utils"

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

// NewDriverImage returns a new DriverImage from an image reference.
// For Docker use `docker://bblfsh/rust-driver:latest`.
func NewDriverImage(imageRef string) (DriverImage, error) {
	ir, err := ParseImageName(imageRef)
	if err != nil {
		return nil, err
	}

	return &driverImage{ref: ir}, nil
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
	if err := utils.UnpackImage(img, path); err != nil {
		return err
	}

	config, err := img.OCIConfig()
	if err != nil {
		return err
	}

	return utils.WriteImageConfig(config, path+".json")
}

func (d *driverImage) image() (types.Image, error) {
	raw, err := d.ref.NewImageSource(nil, nil)
	if err != nil {
		return nil, err
	}

	unparsedImage := image.UnparsedFromSource(raw)
	return image.FromUnparsedImage(unparsedImage)
}
