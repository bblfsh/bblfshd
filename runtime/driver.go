package runtime

import (
	"fmt"

	"github.com/bblfsh/server/utils"

	"github.com/containers/image/docker"
	"github.com/containers/image/image"
	"github.com/containers/image/types"
)

type Driver struct {
	Image DriverImage
}

type DriverImage interface {
	Name() string
	Digest() (Digest, error)
	Inspect() (*types.ImageInspectInfo, error)
	WriteTo(path string) error
}

type driverImage struct {
	ref types.ImageReference
}

func NewDriverImage(imageName string) (DriverImage, error) {
	ref, err := docker.ParseReference(imageName)
	if err != nil {
		return nil, fmt.Errorf("Invalid source name %s: %v", imageName, err)
	}

	return &driverImage{ref: ref}, nil
}

func (d *driverImage) Name() string {
	return d.ref.StringWithinTransport()
}

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
