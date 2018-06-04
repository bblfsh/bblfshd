package daemon

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/containers/image/types"
	oldctx "golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/manifest"
	sdk "gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/driver"
	"gopkg.in/bblfsh/sdk.v1/uast"
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

func (d *mockDriver) Service() sdk.ProtocolServiceClient {
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

func (d *echoDriver) NativeParse(
	_ oldctx.Context, in *sdk.NativeParseRequest, opts ...grpc.CallOption) (*sdk.NativeParseResponse, error) {
	return &sdk.NativeParseResponse{
		AST: in.Content,
	}, nil
}

func (d *echoDriver) Parse(
	_ oldctx.Context, in *sdk.ParseRequest, opts ...grpc.CallOption) (*sdk.ParseResponse, error) {
	return &sdk.ParseResponse{
		UAST: &uast.Node{
			Token: in.Content,
		},
	}, nil
}

func (d *echoDriver) Version(
	_ oldctx.Context, in *sdk.VersionRequest, opts ...grpc.CallOption) (*sdk.VersionResponse, error) {
	return &sdk.VersionResponse{}, nil
}

func (d *echoDriver) SupportedLanguages(
	_ oldctx.Context, in *sdk.SupportedLanguagesRequest, opts ...grpc.CallOption) (*sdk.SupportedLanguagesResponse, error) {
	drivers := []sdk.DriverManifest{sdk.DriverManifest{Name: "Python"}}
	return &sdk.SupportedLanguagesResponse{Languages: drivers}, nil
}

func (d *echoDriver) Service() sdk.ProtocolServiceClient {
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
