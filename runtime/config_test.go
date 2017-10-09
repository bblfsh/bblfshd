package runtime

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestWriteImageConfig(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "")
	require.NoError(err)
	defer os.RemoveAll(dir)

	dir = filepath.Join(dir, "foo")
	err = WriteImageConfig(&ImageConfig{
		ImageRef: "foo",
		Image: v1.Image{
			Author: "foo",
			OS:     "bar",
		},
	}, dir)
	require.NoError(err)

	b, err := ioutil.ReadFile(dir + ".json")
	require.NoError(err)
	require.Equal("{\"image_ref\":\"foo\",\"author\":\"foo\",\"architecture\":\"\",\"os\":\"bar\",\"config\":{},\"rootfs\":{\"type\":\"\",\"diff_ids\":null}}\n", string(b))
}

func TestReadImageConfig(t *testing.T) {
	require := require.New(t)

	dir, err := ioutil.TempDir("", "")
	require.NoError(err)
	defer os.RemoveAll(dir)
	dir = filepath.Join(dir, "qux")

	content := "{\"image_ref\":\"foo\",\"author\":\"foo\",\"architecture\":\"\",\"os\":\"bar\",\"config\":{},\"rootfs\":{\"type\":\"\",\"diff_ids\":null}}\n"
	err = ioutil.WriteFile(dir+".json", []byte(content), 0644)
	require.NoError(err)

	config, err := ReadImageConfig(dir)
	require.NoError(err)

	require.Equal(&ImageConfig{
		ImageRef: "foo",
		Image: v1.Image{
			Author: "foo",
			OS:     "bar",
		},
	}, config)

}
