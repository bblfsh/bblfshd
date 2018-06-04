package daemon

import (
	"github.com/bblfsh/bblfshd/daemon/protocol"
	"github.com/bblfsh/bblfshd/runtime"

	oldctx "golang.org/x/net/context"
	"google.golang.org/grpc"
	sdk "gopkg.in/bblfsh/sdk.v1/protocol"
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
