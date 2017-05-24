package utils

import (
	"encoding/json"
	"os"

	"github.com/opencontainers/image-spec/specs-go/v1"
)

func WriteImageConfig(config *v1.Image, path string) error {
	f, err := os.Create(path)
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

func ReadImageConfig(path string) (*v1.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var cerr error
	defer func() { cerr = f.Close() }()

	dec := json.NewDecoder(f)
	config := &v1.Image{}
	if err := dec.Decode(config); err != nil {
		return nil, err
	}

	return config, cerr
}
