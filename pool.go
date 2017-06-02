package server

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/bblfsh/sdk/protocol"
	"github.com/pkg/errors"
)

var (
	// DefaultPoolTimeout is the time a request to the DriverPool can wait
	// before getting a driver assigned.
	DefaultPoolTimeout = time.Second * 5
)

// DriverPool controls a pool of drivers and balances requests among them,
// ensuring each driver does not get concurrent requests. The number of driver
// instances in the driver pool is controlled by a ScalingPolicy.
type DriverPool struct {
	scalingPolicy ScalingPolicy
	timeout       time.Duration
	new           func() (Driver, error)

	cur int
	// close channel will be used to synchronize Close() call with the
	// scaling() goroutine. Once Close() starts, a struct{} will be sent to
	// the close channel. And once scaling() finish it will close it.
	close      chan struct{}
	closed     bool
	waiting    *atomicInt
	readyQueue *driverQueue
}

// StartDriverPool creates and starts a new DriverPool. It takes as parameters
// the ScalingPolicy, the timeout for requests before getting a driver assigned,
// and a function to create driver instances.
func StartDriverPool(scaling ScalingPolicy, timeout time.Duration, new func() (Driver, error)) (*DriverPool, error) {
	dp := &DriverPool{
		scalingPolicy: scaling,
		timeout:       timeout,
		new:           new,
		close:         make(chan struct{}),
		waiting:       &atomicInt{},
		readyQueue:    newDriverQueue(),
	}

	if err := dp.start(); err != nil {
		return nil, err
	}

	return dp, nil
}

func (dp *DriverPool) start() error {
	target := dp.scalingPolicy.Scale(0, 0)
	if err := dp.setInstanceCount(target); err != nil {
		_ = dp.setInstanceCount(0)
		return err
	}

	go dp.scaling()
	return nil
}

// setInstanceCount changes the number of running driver instances. Instances
// will be started or stopped as necessary to satisfy the new instance count.
// It blocks until the all required instances are started or stopped.
func (dp *DriverPool) setInstanceCount(target int) error {
	if target < 0 {
		return fmt.Errorf("cannot set instances to negative number")
	}

	n := target - dp.cur
	if n > 0 {
		return dp.add(n)
	} else if n < 0 {
		return dp.del(-n)
	}

	return nil
}

func (dp *DriverPool) add(n int) error {
	for i := 0; i < n; i++ {
		d, err := dp.new()
		if err != nil {
			return err
		}

		dp.readyQueue.Enqueue(d)
		dp.cur++
	}

	return nil
}

func (dp *DriverPool) del(n int) error {
	for i := 0; i < n; i++ {
		d, more := dp.readyQueue.Dequeue()
		if !more {
			return fmt.Errorf("driver queue closed")
		}

		dp.cur--
		if err := d.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (dp *DriverPool) scaling() {
	for {
		select {
		case <-dp.close:
			close(dp.close)
			return
		case <-time.After(time.Millisecond * 100):
			total := dp.cur
			ready := dp.readyQueue.Size()
			load := int(dp.waiting.Value())
			s := dp.scalingPolicy.Scale(total, load-ready)
			_ = dp.setInstanceCount(s)
		}
	}
}

// ParseUAST processes a ParseUASTRequest. It gets a driver from the pool and
// forwards the request to it. If all drivers are busy, it will return an error
// after the timeout passes. If the DriverPool is closed, an error will be returned.
func (dp *DriverPool) ParseUAST(req *protocol.ParseUASTRequest) *protocol.ParseUASTResponse {
	dp.waiting.Add(1)
	d, more, timedout := dp.readyQueue.TryDequeue(dp.timeout)
	dp.waiting.Add(-1)

	if !more {
		return &protocol.ParseUASTResponse{
			Status: protocol.Fatal,
			Errors: []string{"driver pool already closed"},
		}
	}

	if timedout {
		return &protocol.ParseUASTResponse{
			Status: protocol.Fatal,
			Errors: []string{"timedout: all drivers are busy"},
		}
	}

	defer dp.readyQueue.Enqueue(d)
	return d.ParseUAST(req)

}

// Close closes the driver pool, including all its underlying driver instances.
func (dp *DriverPool) Close() error {
	if dp.closed {
		return errors.New("already closed")
	}

	dp.closed = true
	dp.close <- struct{}{}
	<-dp.close
	if err := dp.setInstanceCount(0); err != nil {
		return err
	}

	return dp.readyQueue.Close()
}

type driverQueue struct {
	c chan Driver
	n *atomicInt
}

func newDriverQueue() *driverQueue {
	return &driverQueue{c: make(chan Driver), n: &atomicInt{}}
}

func (q *driverQueue) Enqueue(d Driver) {
	q.n.Add(1)
	go func() { q.c <- d }()
}

func (q *driverQueue) Dequeue() (driver Driver, more bool) {
	d, more := <-q.c
	q.n.Add(-1)
	return d, more
}

func (q *driverQueue) TryDequeue(timeout time.Duration) (driver Driver, more, timedout bool) {
	select {
	case d, more := <-q.c:
		q.n.Add(-1)
		return d, more, false
	case <-time.After(timeout):
		return nil, true, true
	}
}

func (q *driverQueue) Size() int {
	return int(q.n.Value())
}

func (q *driverQueue) Close() error {
	close(q.c)
	return nil
}

type atomicInt struct {
	val int32
}

func (c *atomicInt) Add(n int) {
	atomic.AddInt32(&c.val, int32(n))
}

func (c *atomicInt) Value() int {
	return int(atomic.LoadInt32(&c.val))
}

// ScalingPolicy specifies whether instances should be started or stopped to
// cope with load.
type ScalingPolicy interface {
	// Scale takes the number of total instances and the load. The load is
	// the number of request waiting or, there is none, it is a negative
	// value indicating how many instances are ready.
	Scale(total, load int) int
}

// DefaultScalingPolicy returns a new instance of the default scaling policy.
// Instances returned by this function should not be reused.
func DefaultScalingPolicy() ScalingPolicy {
	return MovingAverage(10, MinMax(1, 10, AIMD(1, 0.5)))
}

type movingAverage struct {
	ScalingPolicy
	loads  []float64
	pos    int
	filled bool
}

// MovingAverage computes a moving average of the load and forwards it to the
// underlying scaling policy. This policy is stateful and not thread-safe, do not
// reuse its instances for multiple pools.
func MovingAverage(window int, p ScalingPolicy) ScalingPolicy {
	return &movingAverage{
		ScalingPolicy: p,
		loads:         make([]float64, window),
		pos:           0,
		filled:        false,
	}
}

func (p *movingAverage) Scale(total, load int) int {
	p.loads[p.pos] = float64(load)
	p.pos++
	if p.pos >= len(p.loads) {
		p.filled = true
		p.pos = 0
	}

	maxPos := len(p.loads)
	if !p.filled {
		maxPos = p.pos
	}

	var sum float64
	for i := 0; i < maxPos; i++ {
		sum += p.loads[i]
	}

	avg := sum / float64(maxPos)
	return p.ScalingPolicy.Scale(total, int(avg))
}

type minMax struct {
	ScalingPolicy
	Min, Max int
}

// MinMax wraps a ScalingPolicy and applies a minimum and maximum to the number
// of instances.
func MinMax(min, max int, p ScalingPolicy) ScalingPolicy {
	return &minMax{
		Min:           min,
		Max:           max,
		ScalingPolicy: p,
	}
}

func (p *minMax) Scale(total, load int) int {
	s := p.ScalingPolicy.Scale(total, load)
	if s > p.Max {
		return p.Max
	}

	if s < p.Min {
		return p.Min
	}

	return s
}

type aimd struct {
	Add int
	Mul float64
}

// AIMD returns a ScalingPolicy of additive increase / multiplicative decrease.
// Increases are of min(add, load). Decreases are of (ready / mul).
func AIMD(add int, mul float64) ScalingPolicy {
	return &aimd{add, mul}
}

func (p *aimd) Scale(total, load int) int {
	if load > 0 {
		if load > p.Add {
			return total + p.Add
		}

		return total + load
	}

	res := total - int(math.Ceil(float64(-load)*p.Mul))
	if res < 0 {
		return 0
	}

	return res
}
