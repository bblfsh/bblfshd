package server

import (
	"testing"

	"github.com/bblfsh/sdk/protocol"
	"github.com/stretchr/testify/require"
	"sync"
	"time"
)

type mockDriver struct {
	Response    *protocol.ParseUASTResponse
	Time        time.Duration
	CalledClose int
}

func (d *mockDriver) ParseUAST(req *protocol.ParseUASTRequest) *protocol.ParseUASTResponse {
	time.Sleep(d.Time)
	return d.Response
}

func (d *mockDriver) Close() error {
	d.CalledClose++
	return nil
}

func TestDriverPoolStartNoopClose(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		return &mockDriver{}, nil
	}

	dp := NewDriverPool(new)

	err := dp.Start()
	require.NoError(err)

	err = dp.Close()
	require.NoError(err)

	err = dp.Close()
	require.EqualError(err, "already closed")

	resp := dp.ParseUAST(&protocol.ParseUASTRequest{})
	require.NotNil(resp)
	require.Equal(protocol.Fatal, resp.Status)
	require.Equal([]string{"driver pool already closed"}, resp.Errors)
}

func TestDriverPoolSequential(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		resp := &protocol.ParseUASTResponse{
			Status: protocol.Ok,
		}
		return &mockDriver{
			Response: resp,
			Time:     time.Millisecond * 50,
		}, nil
	}

	dp := NewDriverPool(new)
	err := dp.Start()
	require.NoError(err)

	for i := 0; i < 100; i++ {
		resp := dp.ParseUAST(&protocol.ParseUASTRequest{})
		require.NotNil(resp)
		require.Equal(protocol.Ok, resp.Status)
		require.Equal(dp.Min, dp.cur)
	}

	err = dp.Close()
	require.NoError(err)
}

func TestDriverPoolParallel(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		resp := &protocol.ParseUASTResponse{
			Status: protocol.Ok,
		}
		return &mockDriver{
			Response: resp,
			Time:     time.Millisecond * 100,
		}, nil
	}

	dp := NewDriverPool(new)
	dp.TimeBeforeNew = time.Millisecond * 20
	dp.TimeBetweenSpawns = time.Millisecond * 80
	dp.TimeBeforeClean = time.Second * 200
	err := dp.Start()
	require.NoError(err)

	wg := &sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			resp := dp.ParseUAST(&protocol.ParseUASTRequest{})
			require.NotNil(resp)
			require.Equal(protocol.Ok, resp.Status)
			require.True(dp.cur >= dp.Min)
			wg.Done()
		}()
	}

	wg.Wait()
	require.Equal(dp.Max, dp.cur)

	dp.TimeBeforeClean = time.Millisecond * 2
	time.Sleep(time.Second * 2)
	require.Equal(dp.Min, dp.cur)

	err = dp.Close()
	require.NoError(err)
}
