package utils

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestWriteImageConfig(t *testing.T) {
	require := require.New(t)

	f, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(err)
	path := f.Name()
	require.NoError(f.Close())

	err = WriteImageConfig(&v1.Image{
		Author: "foo",
		OS:     "bar",
	}, path)
	require.NoError(err)

	b, err := ioutil.ReadFile(path)
	require.NoError(err)
	require.Equal("{\"author\":\"foo\",\"architecture\":\"\",\"os\":\"bar\",\"config\":{},\"rootfs\":{\"type\":\"\",\"diff_ids\":null}}\n", string(b))
}

func TestReadImageConfig(t *testing.T) {
	require := require.New(t)

	f, err := ioutil.TempFile(os.TempDir(), "test")
	require.NoError(err)
	path := f.Name()
	require.NoError(f.Close())

	content := "{\"author\":\"foo\",\"architecture\":\"\",\"os\":\"bar\",\"config\":{},\"rootfs\":{\"type\":\"\",\"diff_ids\":null}}\n"
	err = ioutil.WriteFile(path, []byte(content), 0644)
	require.NoError(err)

	config, err := ReadImageConfig(path)
	require.NoError(err)

	require.Equal(&v1.Image{
		Author: "foo",
		OS:     "bar",
	}, config)

}
