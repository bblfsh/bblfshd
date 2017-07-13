package server

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bblfsh/sdk/protocol"
	"github.com/stretchr/testify/require"
)

type mockDriver struct {
	Response    *protocol.ParseResponse
	Time        time.Duration
	CalledClose int
}

func (d *mockDriver) Parse(req *protocol.ParseRequest) *protocol.ParseResponse {
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

	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, new)
	require.NoError(err)
	require.NotNil(dp)

	err = dp.Close()
	require.NoError(err)

	err = dp.Close()
	require.EqualError(err, "already closed")

	resp := dp.Parse(&protocol.ParseRequest{})
	require.NotNil(resp)
	require.Equal(protocol.Fatal, resp.Status)
	require.Equal([]string{"driver pool already closed"}, resp.Errors)
}

func TestDriverPoolStartFailingDriver(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		return nil, fmt.Errorf("driver error")
	}

	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, new)
	require.EqualError(err, "driver error")
	require.Nil(dp)
}

func TestDriverPoolSequential(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		resp := &protocol.ParseResponse{
			Status: protocol.Ok,
		}
		return &mockDriver{
			Response: resp,
			Time:     time.Millisecond * 50,
		}, nil
	}

	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, new)
	require.NoError(err)
	require.NotNil(dp)

	for i := 0; i < 100; i++ {
		resp := dp.Parse(&protocol.ParseRequest{})
		require.NotNil(resp)
		require.Equal(protocol.Ok, resp.Status)
		//FIXME: it should be always 1
		require.True(dp.cur == 1 || dp.cur == 2)
	}

	err = dp.Close()
	require.NoError(err)
}

func TestDriverPoolParallel(t *testing.T) {
	require := require.New(t)

	new := func() (Driver, error) {
		resp := &protocol.ParseResponse{
			Status: protocol.Ok,
		}
		return &mockDriver{
			Response: resp,
			Time:     time.Millisecond * 100,
		}, nil
	}

	dp, err := StartDriverPool(DefaultScalingPolicy(), time.Second*10, new)
	require.NoError(err)
	require.NotNil(dp)

	wg := &sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			resp := dp.Parse(&protocol.ParseRequest{})
			wg.Done()
			require.NotNil(resp)
			require.Nil(resp.Errors)
			require.Equal(protocol.Ok, resp.Status)
			require.True(dp.cur >= 1)
		}()
	}

	wg.Wait()
	require.Equal(runtime.NumCPU(), dp.cur)

	time.Sleep(time.Second * 2)
	require.Equal(1, dp.cur)

	err = dp.Close()
	require.NoError(err)
}

type mockScalingPolicy struct {
	Total, Load int
	Result      int
}

func (p *mockScalingPolicy) Scale(total int, load int) int {
	p.Total = total
	p.Load = load
	return p.Result
}

func TestMinMax(t *testing.T) {
	require := require.New(t)

	m := &mockScalingPolicy{}
	p := MinMax(5, 10, m)
	m.Result = 1
	require.Equal(5, p.Scale(1, 1))
	m.Result = 5
	require.Equal(5, p.Scale(1, 1))
	m.Result = 7
	require.Equal(7, p.Scale(1, 1))
	m.Result = 10
	require.Equal(10, p.Scale(1, 1))
	m.Result = 11
	require.Equal(10, p.Scale(1, 1))
}

func TestMovingAverage(t *testing.T) {
	require := require.New(t)

	m := &mockScalingPolicy{}
	p := MovingAverage(1, m)
	p.Scale(1, 2)
	require.Equal(1, m.Total)
	require.Equal(2, m.Load)
	p.Scale(1, 50)
	require.Equal(1, m.Total)
	require.Equal(50, m.Load)

	p = MovingAverage(2, m)
	p.Scale(1, 1)
	require.Equal(1, m.Load)
	p.Scale(1, 3)
	require.Equal(2, m.Load)
	p.Scale(1, 7)
	require.Equal(5, m.Load)

	p = MovingAverage(100, m)
	for i := 0; i < 100; i++ {
		p.Scale(1, 200)
		require.Equal(200, m.Load)
	}

	for i := 0; i < 50; i++ {
		p.Scale(1, 100)
	}
	require.Equal(150, m.Load)
}

func TestAIMD(t *testing.T) {
	require := require.New(t)

	p := AIMD(1, 0.5)

	require.Equal(0, p.Scale(0, 0))
	require.Equal(1, p.Scale(1, 0))

	require.Equal(1, p.Scale(0, 1))
	require.Equal(2, p.Scale(1, 1))

	require.Equal(0, p.Scale(1, -1))
	require.Equal(1, p.Scale(2, -1))
}
