package daemon

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/types"
	xcontext "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/runtime"
	"gopkg.in/bblfsh/sdk.v1/manifest"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/driver"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
	"gopkg.in/bblfsh/sdk.v2/uast"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes/nodesproto"
)

type mockDriver struct {
	CalledClose int
	MockID      string
	MockStatus  protocol.Status
}

func newMockDriver() (Driver, error) {
	return &mockDriver{
		MockID:     runtime.NewULID().String(),
		MockStatus: protocol.Running,
	}, nil
}

func (d *mockDriver) ID() string {
	return d.MockID
}

func (d *mockDriver) Service() ServiceV1 {
	return nil
}

func (d *mockDriver) ServiceV2() protocol2.DriverClient {
	return nil
}

func (d *mockDriver) Start() error {
	return nil
}

func (d *mockDriver) Status() (protocol.Status, error) {
	return d.MockStatus, nil
}

func (d *mockDriver) State() (*protocol.DriverInstanceState, error) {
	return nil, nil
}

func (d *mockDriver) Stop() error {
	d.CalledClose++
	return nil
}

func newEchoDriver() *echoDriver {
	d, _ := newMockDriver()
	return &echoDriver{
		Driver: d,
	}
}

type echoDriver struct {
	Driver
}

func (d *echoDriver) Parse(ctx xcontext.Context, in *protocol2.ParseRequest, opts ...grpc.CallOption) (*protocol2.ParseResponse, error) {
	buf := bytes.NewBuffer(nil)
	err := nodesproto.WriteTo(buf, nodes.Object{
		uast.KeyToken: nodes.String(in.Content),
	})
	if err != nil {
		return nil, err
	}
	return &protocol2.ParseResponse{
		Uast: buf.Bytes(),
	}, nil
}

func (d *echoDriver) Version(
	_ xcontext.Context, in *protocol1.VersionRequest, opts ...grpc.CallOption) (*protocol1.VersionResponse, error) {
	return &protocol1.VersionResponse{}, nil
}

func (d *echoDriver) SupportedLanguages(
	_ xcontext.Context, in *protocol1.SupportedLanguagesRequest, opts ...grpc.CallOption) (*protocol1.SupportedLanguagesResponse, error) {
	drivers := []protocol1.DriverManifest{protocol1.DriverManifest{Name: "Python"}}
	return &protocol1.SupportedLanguagesResponse{Languages: drivers}, nil
}

func (d *echoDriver) Service() ServiceV1 {
	return d
}

func (d *echoDriver) ServiceV2() protocol2.DriverClient {
	return d
}

func newMockDriverImage(lang string) runtime.DriverImage {
	return &mockDriverImage{lang: lang}
}

type mockDriverImage struct {
	lang string
}

func (d *mockDriverImage) Name() string {
	return d.lang
}

func (d *mockDriverImage) Digest() (runtime.Digest, error) {
	return runtime.NewDigest(hex.EncodeToString([]byte(d.Name()))), nil
}

func (d *mockDriverImage) Inspect() (*types.ImageInspectInfo, error) {
	return &types.ImageInspectInfo{}, nil
}

func (d *mockDriverImage) WriteTo(path string) error {
	if err := writeManifest(d.Name(), path); err != nil {
		return err
	}

	return writeImageConfig(d.Name(), path)
}

func writeManifest(language, path string) error {
	manifest := manifest.Manifest{
		Name: language,
	}

	manifestPath := filepath.Join(path, driver.ManifestLocation)
	manifestBaseDir := filepath.Dir(manifestPath)
	if err := os.MkdirAll(manifestBaseDir, 0755); err != nil {
		return err
	}

	manifestFile, err := os.Create(manifestPath)
	if err != nil {
		return err
	}
	defer manifestFile.Close()

	return manifest.Encode(manifestFile)
}

func writeImageConfig(language, path string) error {
	return runtime.WriteImageConfig(&runtime.ImageConfig{
		ImageRef: fmt.Sprintf("%s-driver", language),
	}, path)
}
