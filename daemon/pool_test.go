package daemon

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriverPoolClose_StartNoopClose(t *testing.T) {
	require := require.New(t)
	dp := NewDriverPool(newMockDriver)

	ctx := context.Background()
	err := dp.Start(ctx)
	require.NoError(err)

	err = dp.Stop()
	require.NoError(err)

	err = dp.Stop()
	require.True(ErrPoolClosed.Is(err), "%v", err)

	err = dp.ExecuteCtx(ctx, func(ctx context.Context, d Driver) error {
		return errors.New("should not happen")
	})
	require.True(ErrPoolClosed.Is(err), "%v", err)
}

func TestDriverPoolCurrent(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(newMockDriver)

	err := dp.Start(context.Background())
	require.NoError(err)

	require.Len(dp.Current(), 1)

	err = dp.Stop()
	require.NoError(err)
}

func TestDriverPoolExecute_Timeout(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(func(ctx context.Context) (Driver, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond):
		}
		return newMockDriver(ctx)
	})

	err := dp.Start(context.Background())
	require.NoError(err)
	defer dp.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	err = dp.ExecuteCtx(ctx, func(ctx context.Context, d Driver) error {
		return errors.New("should not happen")
	})
	require.True(err == context.DeadlineExceeded)
}

func TestDriverPoolState(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	dp := NewDriverPool(newMockDriver)

	err := dp.Start(context.Background())
	require.NoError(err)
	assert.Equal(1, dp.State().Wanted)
	assert.Equal(1, dp.State().Running)

	err = dp.Stop()
	require.NoError(err)
	assert.Equal(0, dp.State().Wanted)
	assert.Equal(0, dp.State().Running)

}

func TestDiverPoolStart_FailingDriver(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(func(ctx context.Context) (Driver, error) {
		return nil, fmt.Errorf("driver error")
	})

	err := dp.Start(context.Background())
	require.EqualError(err, "driver error")
	err = dp.Stop()
	require.True(ErrPoolClosed.Is(err))
}

func TestDriverPoolExecute_Recovery(t *testing.T) {
	require := require.New(t)

	var called int
	dp := NewDriverPool(func(ctx context.Context) (Driver, error) {
		called++
		return newMockDriver(ctx)
	})

	ctx := context.Background()

	err := dp.Start(ctx)
	require.NoError(err)

	for i := 0; i < 100; i++ {
		err := dp.ExecuteCtx(ctx, func(_ context.Context, d Driver) error {
			require.NotNil(d)

			if i%10 == 9 {
				d.(*mockDriver).MockStatus = protocol.Stopped
			}

			return nil
		})

		require.Nil(err)
		if i%10 != 9 {
			require.Len(dp.Current(), 1)
		}
	}

	err = dp.Stop()
	require.NoError(err)
	require.Equal(dp.State().Success, 100)
	require.Equal(dp.State().Exited, 10)
	require.Equal(dp.State().Wanted, 0)
}

func TestDriverPoolExecute_Sequential(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(newMockDriver)

	ctx := context.Background()

	err := dp.Start(ctx)
	require.NoError(err)

	for i := 0; i < 100; i++ {
		err := dp.ExecuteCtx(ctx, func(_ context.Context, d Driver) error {
			require.NotNil(d)
			return nil
		})

		require.Nil(err)
		require.Equal(dp.State().Running, 1)
	}

	err = dp.Stop()
	require.NoError(err)
}

func TestDriverPoolExecute_Parallel(t *testing.T) {
	require := require.New(t)

	oldWindow := policyDefaultWindow
	defer func() {
		policyDefaultWindow = oldWindow
	}()
	policyDefaultWindow = time.Second / 2

	dp := NewDriverPool(newMockDriver)

	ctx := context.Background()

	err := dp.Start(ctx)
	require.NoError(err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := dp.ExecuteCtx(ctx, func(_ context.Context, _ Driver) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(policyDefaultTick / 2):
				}
				return nil
			})

			require.Nil(err)
			require.True(len(dp.Current()) >= 1)
		}()
	}

	wg.Wait()
	require.Len(dp.Current(), runtime.NumCPU())

	// need approximately two full windows, times the inverse downscale factor
	time.Sleep(policyDefaultWindow * defaultPolicyTargetWindow * time.Duration(1/policyDefaultDownscale))
	require.Equal(1, dp.State().Running)

	err = dp.Stop()
	require.NoError(err)
}

type mockScalingPolicy struct {
	Total, Idle, Load int
	Result            int
}

func (p *mockScalingPolicy) Scale(total, idle, load int) int {
	p.Total = total
	p.Idle = idle
	p.Load = load
	return p.Result
}

func TestMinMax(t *testing.T) {
	require := require.New(t)

	m := &mockScalingPolicy{}
	p := MinMax(5, 10, m)
	m.Result = 1
	require.Equal(5, p.Scale(1, 0, 1))
	m.Result = 5
	require.Equal(5, p.Scale(1, 0, 1))
	m.Result = 7
	require.Equal(7, p.Scale(1, 0, 1))
	m.Result = 10
	require.Equal(10, p.Scale(1, 0, 1))
	m.Result = 11
	require.Equal(10, p.Scale(1, 0, 1))
}

func TestMovingAverage(t *testing.T) {
	require := require.New(t)

	m := &mockScalingPolicy{}
	p := MovingAverage(1, m)
	p.Scale(1, 0, 2)
	require.Equal(1, m.Total)
	require.Equal(2, m.Load)
	p.Scale(1, 0, 50)
	require.Equal(1, m.Total)
	require.Equal(50, m.Load)

	p = MovingAverage(2, m)
	p.Scale(1, 0, 1)
	require.Equal(1, m.Load)
	p.Scale(1, 0, 3)
	require.Equal(2, m.Load)
	p.Scale(1, 0, 7)
	require.Equal(5, m.Load)

	p = MovingAverage(100, m)
	for i := 0; i < 100; i++ {
		p.Scale(1, 0, 200)
		require.Equal(200, m.Load)
	}

	for i := 0; i < 50; i++ {
		p.Scale(1, 0, 100)
	}
	require.Equal(150, m.Load)
}

func TestAIMD(t *testing.T) {
	require := require.New(t)

	p := AIMD(1, 0.5)

	require.Equal(1, p.Scale(0, 0, 0))
	require.Equal(1, p.Scale(1, 0, 0))
	require.Equal(1, p.Scale(1, 1, 0))

	require.Equal(1, p.Scale(0, 0, 1))
	require.Equal(2, p.Scale(1, 0, 1))
	require.Equal(2, p.Scale(1, 1, 2))

	require.Equal(1, p.Scale(1, 1, 0))
	require.Equal(1, p.Scale(2, 2, 1))
	require.Equal(2, p.Scale(2, 2, 2))
}
