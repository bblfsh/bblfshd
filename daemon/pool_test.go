package daemon

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bblfsh/server/daemon/protocol"
	"github.com/stretchr/testify/require"
)

func TestDriverPoolClose_StartNoopClose(t *testing.T) {
	require := require.New(t)
	dp := NewDriverPool(newMockDriver)

	err := dp.Start()
	require.NoError(err)

	err = dp.Stop()
	require.NoError(err)

	err = dp.Stop()
	require.True(ErrPoolClosed.Is(err))

	err = dp.Execute(nil, 0)
	require.True(ErrPoolClosed.Is(err))
}

func TestDriverPoolCurrent(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(newMockDriver)

	err := dp.Start()
	require.NoError(err)

	require.Len(dp.Current(), 1)
}

func TestDriverPoolExecute_Timeout(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(func() (Driver, error) {
		time.Sleep(time.Millisecond)
		return newMockDriver()
	})

	err := dp.Execute(nil, time.Nanosecond)
	require.True(ErrPoolTimeout.Is(err))
}

func TestDriverPoolExecute_InvalidTimeout(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(func() (Driver, error) {
		time.Sleep(time.Millisecond)
		return newMockDriver()
	})

	err := dp.Execute(nil, 100*time.Minute)
	require.True(ErrInvalidPoolTimeout.Is(err))
}

func TestDriverPoolState(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(newMockDriver)

	err := dp.Start()
	require.NoError(err)
	require.Equal(dp.State().Wanted, 1)
	require.Equal(dp.State().Running, 1)

	err = dp.Stop()
	require.NoError(err)
	require.Equal(dp.State().Wanted, 0)
	require.Equal(dp.State().Running, 0)

}

func TestDiverPoolStart_FailingDriver(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(func() (Driver, error) {
		return nil, fmt.Errorf("driver error")
	})

	err := dp.Start()
	require.EqualError(err, "driver error")
}

func TestDriverPoolExecute_Recovery(t *testing.T) {
	require := require.New(t)

	var called int
	dp := NewDriverPool(func() (Driver, error) {
		called++
		return newMockDriver()
	})

	err := dp.Start()
	require.NoError(err)

	for i := 0; i < 100; i++ {
		err := dp.Execute(func(d Driver) error {
			require.NotNil(d)

			if i%10 == 0 {
				d.(*mockDriver).MockStatus = protocol.Stopped
			}

			return nil
		}, 0)

		require.Nil(err)
		require.Len(dp.Current(), 1)
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

	err := dp.Start()
	require.NoError(err)

	for i := 0; i < 100; i++ {
		err := dp.Execute(func(d Driver) error {
			require.NotNil(d)
			return nil
		}, 0)

		require.Nil(err)
		require.Equal(dp.State().Running, 1)
	}

	err = dp.Stop()
	require.NoError(err)
}

func TestDriverPoolExecute_Parallel(t *testing.T) {
	require := require.New(t)

	dp := NewDriverPool(newMockDriver)

	err := dp.Start()
	require.NoError(err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			err := dp.Execute(func(Driver) error {
				defer wg.Done()
				time.Sleep(50 * time.Millisecond)
				return nil
			}, 0)

			require.Nil(err)
			require.True(len(dp.Current()) >= 1)
		}()
	}

	wg.Wait()
	require.Len(dp.Current(), runtime.NumCPU())

	time.Sleep(time.Second)
	require.Equal(dp.State().Running, 1)

	err = dp.Stop()
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
