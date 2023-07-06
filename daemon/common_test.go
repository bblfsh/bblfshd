package daemon

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/v2/runtime"

	protocol2 "github.com/bblfsh/sdk/v3/protocol"
	"github.com/containers/image/types"
	oldctx "golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/manifest"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/driver"
	"gopkg.in/bblfsh/sdk.v1/uast"
)

type mockDriver struct {
	CalledClose int
	MockID      string
	MockStatus  protocol.Status
}

func newMockDriver(ctx context.Context) (Driver, error) {
	return &mockDriver{
		MockID:     runtime.NewULID().String(),
		MockStatus: protocol.Running,
	}, nil
}

func (d *mockDriver) ID() string {
	return d.MockID
}

func (d *mockDriver) Service() protocol1.ProtocolServiceClient {
	return nil
}

func (d *mockDriver) ServiceV2() protocol2.DriverClient {
	return nil
}

func (d *mockDriver) Start(ctx context.Context) error {
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
	d, _ := newMockDriver(context.Background())
	return &echoDriver{
		Driver: d,
	}
}

type echoDriver struct {
	Driver
}

func (d *echoDriver) NativeParse(
	_ oldctx.Context, in *protocol1.NativeParseRequest, opts ...grpc.CallOption) (*protocol1.NativeParseResponse, error) {
	return &protocol1.NativeParseResponse{
		AST: in.Content,
	}, nil
}

func (d *echoDriver) Parse(
	_ oldctx.Context, in *protocol1.ParseRequest, opts ...grpc.CallOption) (*protocol1.ParseResponse, error) {
	return &protocol1.ParseResponse{
		UAST: &uast.Node{
			Token: in.Content,
		},
	}, nil
}

func (d *echoDriver) Version(
	_ oldctx.Context, in *protocol1.VersionRequest, opts ...grpc.CallOption) (*protocol1.VersionResponse, error) {
	return &protocol1.VersionResponse{}, nil
}

func (d *echoDriver) SupportedLanguages(
	_ oldctx.Context, in *protocol1.SupportedLanguagesRequest, opts ...grpc.CallOption) (*protocol1.SupportedLanguagesResponse, error) {
	drivers := []protocol1.DriverManifest{protocol1.DriverManifest{Name: "Python"}}
	return &protocol1.SupportedLanguagesResponse{Languages: drivers}, nil
}

func (d *echoDriver) Service() protocol1.ProtocolServiceClient {
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
