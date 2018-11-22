package protocol

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/distribution/registry/api/errcode"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockedServiceAnyOtherErr struct {
	mock.Mock
	Service
}

func (s *mockedServiceAnyOtherErr) InstallDriver(language string, image string, update bool) error {
	return errcode.Errors{errors.New("any other error")}
}

func TestServiceMockDaemon_InstallDriverFails(t *testing.T) {
	require := require.New(t)
	//given
	s := new(mockedServiceAnyOtherErr)
	ps := &protocolServiceServer{s}

	//when
	res, err := ps.InstallDriver(context.Background(), &InstallDriverRequest{})

	//then
	require.Nil(res)
	require.Error(err)
}

type mockedServiceUnauthorizedErr struct {
	mock.Mock
	Service
}

func (s *mockedServiceUnauthorizedErr) InstallDriver(language string, image string, update bool) error {
	return errcode.Errors{
		errcode.Error{Code: errcode.ErrorCodeDenied},
		errcode.Error{Code: errcode.ErrorCodeUnauthorized},
	}
}

func TestServiceMockDaemon_InstallNonexistentDriver(t *testing.T) {
	require := require.New(t)
	//given
	s := new(mockedServiceUnauthorizedErr)
	ps := &protocolServiceServer{s}

	//when
	res, err := ps.InstallDriver(context.Background(), &InstallDriverRequest{})

	//then
	require.Nil(res)
	require.Error(err)

	st, ok := status.FromError(err)
	require.True(ok)

	require.Equal(codes.Unauthenticated, st.Code())
}
