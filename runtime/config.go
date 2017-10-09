package runtime

import (
	"encoding/json"
	"os"

	"github.com/opencontainers/image-spec/specs-go/v1"
)

const configExt = ".json"

// ImageConfig describes some basic information about the image.
type ImageConfig struct {
	// ImageRef is the original image reference used to retrieve the image.
	ImageRef string `json:"image_ref"`
	v1.Image
}

func WriteImageConfig(config *ImageConfig, path string) error {
	f, err := os.Create(path + configExt)
	if err != nil {
		return err
	}

	var cerr error
	defer func() { cerr = f.Close() }()

	enc := json.NewEncoder(f)
	if err := enc.Encode(config); err != nil {
		return err
	}

	return cerr
}

func ReadImageConfig(path string) (*ImageConfig, error) {
	f, err := os.Open(path + configExt)
	if err != nil {
		return nil, err
	}

	var cerr error
	defer func() { cerr = f.Close() }()

	dec := json.NewDecoder(f)
	config := &ImageConfig{}
	if err := dec.Decode(config); err != nil {
		return nil, err
	}

	return config, cerr
}
