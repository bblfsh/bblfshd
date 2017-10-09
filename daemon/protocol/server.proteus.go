package protocol

import (
	xcontext "golang.org/x/net/context"
)

type protocolServiceServer struct {
}

func NewProtocolServiceServer() *protocolServiceServer {
	return &protocolServiceServer{}
}
func (s *protocolServiceServer) DriverInstanceStates(ctx xcontext.Context, in *DriverInstanceStatesRequest) (result *DriverInstanceStatesResponse, err error) {
	result = new(DriverInstanceStatesResponse)
	result = DriverInstanceStates()
	return
}
func (s *protocolServiceServer) DriverPoolStates(ctx xcontext.Context, in *DriverPoolStatesRequest) (result *DriverPoolStatesResponse, err error) {
	result = new(DriverPoolStatesResponse)
	result = DriverPoolStates()
	return
}
func (s *protocolServiceServer) DriverStates(ctx xcontext.Context, in *DriverStatesRequest) (result *DriverStatesResponse, err error) {
	result = new(DriverStatesResponse)
	result = DriverStates()
	return
}
func (s *protocolServiceServer) InstallDriver(ctx xcontext.Context, in *InstallDriverRequest) (result *Response, err error) {
	result = new(Response)
	result = InstallDriver(in)
	return
}
func (s *protocolServiceServer) RemoveDriver(ctx xcontext.Context, in *RemoveDriverRequest) (result *Response, err error) {
	result = new(Response)
	result = RemoveDriver(in)
	return
}
