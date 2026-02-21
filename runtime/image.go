package runtime

import (
	"context"
	"fmt"
	"strings"

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
	imageRef string
	ref      types.ImageReference
}

// NewDriverImage returns a new DriverImage from an image reference.
// For Docker use `docker://bblfsh/rust-driver:latest`.
func NewDriverImage(imageRef string) (DriverImage, error) {
	ref, err := ParseImageName(imageRef)
	if err != nil {
		return nil, err
	}

	return &driverImage{
		imageRef: imageRef,
		ref:      ref,
	}, nil
}

// Name returns the name of the driver image based on the image reference.
func (d *driverImage) Name() string {
	return strings.TrimPrefix(d.ref.StringWithinTransport(), "//")
}

// Digest computes a digest based on the image layers.
func (d *driverImage) Digest() (Digest, error) {
	ctx := context.Background()
	src, img, err := d.image(ctx)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	i, err := img.Inspect(ctx)
	if err != nil {
		return nil, err
	}

	return ComputeDigest(i.Layers...), nil
}

func (d *driverImage) Inspect() (*types.ImageInspectInfo, error) {
	ctx := context.Background()
	src, img, err := d.image(ctx)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	return img.Inspect(ctx)
}

// WriteTo writes the image to disk at the given path.
func (d *driverImage) WriteTo(path string) error {
	ctx := context.Background()
	src, img, err := d.image(ctx)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := UnpackImage(img, path); err != nil {
		return fmt.Errorf("failed to unpack image: %w", err)
	}

	config, err := img.OCIConfig(ctx)
	if err != nil {
		return err
	}

	return WriteImageConfig(&ImageConfig{
		Image:    *config,
		ImageRef: d.imageRef,
	}, path)
}

func (d *driverImage) image(ctx context.Context) (types.ImageSource, types.Image, error) {
	src, err := d.ref.NewImageSource(ctx, nil)
	if err != nil {
		return nil, nil, err
	}

	unparsedImage := image.UnparsedInstance(src, nil)
	img, err := image.FromUnparsedImage(ctx, nil, unparsedImage)
	if err != nil {
		src.Close()
		return nil, nil, err
	}
	return src, img, nil
}
