package daemon

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bblfsh/bblfshd/daemon/protocol"

	"github.com/sirupsen/logrus"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	"gopkg.in/src-d/go-errors.v1"
)

const (
	// DefaultPoolTimeout is the time a request to the DriverPool can wait
	// before getting a driver assigned.
	DefaultPoolTimeout = 5 * time.Second
	// MaxPoolTimeout maximum time allowed to wait for a driver be assigned.
	MaxPoolTimeout = 5 * time.Minute
)

var (
	// DefaultMaxInstancesPerDriver is the maximum number of instances of
	// the same driver which can be launched following the default
	// scaling policy (see DefaultScalingPolicy()).
	DefaultMaxInstancesPerDriver = runtime.NumCPU()

	ErrPoolClosed         = errors.NewKind("driver pool already closed")
	ErrPoolTimeout        = errors.NewKind("timeout, all drivers are busy")
	ErrInvalidPoolTimeout = errors.NewKind(fmt.Sprintf("invalid timeout, max. timeout allowed %s", MaxPoolTimeout))
	ErrNegativeInstances  = errors.NewKind("cannot set instances to negative number")
)

// DriverPool controls a pool of drivers and balances requests among them,
// ensuring each driver does not get concurrent requests. The number of driver
// instances in the driver pool is controlled by a ScalingPolicy.
type DriverPool struct {
	// ScalingPolicy scaling policy used to scale up the instances.
	ScalingPolicy ScalingPolicy
	// Timeout time for wait until a driver instance is available.
	Timeout time.Duration
	// Logger used during the live of the driver pool.
	Logger server.Logger

	// factory function used to spawn new driver instances.
	factory FactoryFunction
	// queue holds all the driver instances.
	queue *driverQueue
	// index holds a pointer to the current driver instances by id.
	index sync.Map
	// close channel will be used to synchronize Close() call with the
	// scaling() goroutine. Once Close() starts, a struct{} will be sent to
	// the close channel. And once scaling() finish it will close it.
	close   chan struct{}
	closed  bool
	running bool
	// stats hold different metrics about the state of the pool.
	stats struct {
		instances atomicInt // instances wanted
		waiting   atomicInt // requests waiting for a driver
		success   atomicInt // requests executed successfully
		errors    atomicInt // requests errored
		exited    atomicInt // drivers exited unexpectedly
	}
}

// FactoryFunction is a factory function that creates new DriverInstance's.
type FactoryFunction func() (Driver, error)

// NewDriverPool creates and starts a new DriverPool. It takes as parameters
// a FactoryFunction, used to instantiate new drivers.
func NewDriverPool(factory FactoryFunction) *DriverPool {
	return &DriverPool{
		ScalingPolicy: DefaultScalingPolicy(),
		Timeout:       DefaultPoolTimeout,
		Logger:        logrus.New(),

		factory: factory,
		close:   make(chan struct{}),
		queue:   newDriverQueue(),
	}
}

// Start stats the driver pool.
func (dp *DriverPool) Start() error {
	target := dp.ScalingPolicy.Scale(0, 0)
	if err := dp.setInstances(target); err != nil {
		_ = dp.setInstances(0)
		return err
	}
	dp.running = true

	go dp.scaling()
	return nil
}

// setInstances changes the number of running driver instances. Instances
// will be started or stopped as necessary to satisfy the new instance count.
// It blocks until the all required instances are started or stopped.
func (dp *DriverPool) setInstances(target int) error {
	if target < 0 {
		return ErrNegativeInstances.New()
	}

	n := target - dp.stats.instances.Value()
	if n > 0 {
		return dp.add(n)
	} else if n < 0 {
		return dp.del(-n)
	}

	return nil
}

func (dp *DriverPool) add(n int) error {
	for i := 0; i < n; i++ {
		d, err := dp.factory()
		if err != nil {
			return err
		}

		dp.index.Store(d.ID(), d)
		dp.queue.Put(d)
		dp.stats.instances.Add(1)
	}

	return nil
}

func (dp *DriverPool) del(n int) error {
	for i := 0; i < n; i++ {
		d, more := dp.queue.Get()
		if !more {
			return ErrPoolClosed.New()
		}

		if err := dp.remove(d); err != nil {
			return err
		}
	}

	return nil
}

func (dp *DriverPool) remove(d Driver) error {
	dp.stats.instances.Add(-1)
	if err := d.Stop(); err != nil {
		return err
	}

	dp.index.Delete(d.ID())
	return nil
}

func (dp *DriverPool) scaling() {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-dp.close:
			close(dp.close)
			return
		case <-ticker.C:
			dp.doScaling()
		}
	}
}

func (dp *DriverPool) doScaling() {
	total := dp.stats.instances.Value()
	ready := dp.queue.Size()
	load := dp.stats.waiting.Value()

	s := dp.ScalingPolicy.Scale(total, load-ready)
	if s == total {
		return
	}

	dp.Logger.Debugf("scaling driver pool from %d instance(s) to %d instance(s)", total, s)
	if err := dp.setInstances(s); err != nil {
		dp.Logger.Errorf("error re-scaling pool: %s", err)
	}
}

// Function is a function to be executed using a given driver.
type Function func(d Driver) error

// Execute executes the given Function in the first available driver instance.
// It gets a driver from the pool and forwards the request to it. If all drivers
// are busy, it will return an error after the timeout passes. If the DriverPool
// is closed, an error will be returned.
func (dp *DriverPool) Execute(c Function, timeout time.Duration) error {
	if timeout == 0 {
		timeout = DefaultPoolTimeout
	}

	d, err := dp.getDriver(timeout)
	if err != nil {
		return err
	}

	status, err := d.Status()
	if err != nil {
		return err
	}

	if status != protocol.Running {
		defer func() {
			dp.stats.exited.Add(1)
			dp.Logger.Debugf("removing stopped driver")
			if err := dp.remove(d); err != nil {
				dp.Logger.Errorf("error removing stopped driver: %s", err)
			}
		}()

		return dp.Execute(c, timeout)
	}

	defer dp.queue.Put(d)
	if err := c(d); err != nil {
		dp.stats.errors.Add(1)
		return err
	}

	dp.stats.success.Add(1)
	return nil
}

func (dp *DriverPool) getDriver(timeout time.Duration) (Driver, error) {
	dp.stats.waiting.Add(1)
	defer dp.stats.waiting.Add(-1)

	d, more, err := dp.queue.GetWithTimeout(timeout)
	if err != nil {
		dp.stats.errors.Add(1)
		dp.Logger.Warningf("unable to allocate a driver instance: %s", err)
		return nil, err
	}

	if !more {
		return nil, ErrPoolClosed.New()
	}

	return d, nil
}

// Current returns a list of the current instances from the pool, it includes
// the running ones and those being stopped.
func (dp *DriverPool) Current() []Driver {
	var list []Driver
	dp.index.Range(func(_, d interface{}) bool {
		list = append(list, d.(Driver))
		return true
	})

	return list
}

// State current state of driver pool.
func (dp *DriverPool) State() *protocol.DriverPoolState {
	return &protocol.DriverPoolState{
		Wanted:  dp.stats.instances.Value(),
		Running: len(dp.Current()),
		Waiting: dp.stats.waiting.Value(),
		Success: dp.stats.success.Value(),
		Errors:  dp.stats.errors.Value(),
		Exited:  dp.stats.exited.Value(),
	}
}

// Stop stop the driver pool, including all its underlying driver instances.
func (dp *DriverPool) Stop() error {
	if dp.closed {
		return ErrPoolClosed.New()
	} else if !dp.running {
		return nil
	}

	dp.running = false
	dp.closed = true
	dp.close <- struct{}{}
	<-dp.close
	if err := dp.setInstances(0); err != nil {
		return err
	}

	return dp.queue.Close()
}

type driverQueue struct {
	c chan Driver
	n *atomicInt
}

func newDriverQueue() *driverQueue {
	return &driverQueue{c: make(chan Driver), n: &atomicInt{}}
}

func (q *driverQueue) Put(d Driver) {
	q.n.Add(1)
	go func() { q.c <- d }()
}

func (q *driverQueue) Get() (driver Driver, more bool) {
	defer q.n.Add(-1)

	d, more := <-q.c
	return d, more
}

func (q *driverQueue) GetWithTimeout(timeout time.Duration) (driver Driver, more bool, err error) {
	if timeout > MaxPoolTimeout {
		return nil, true, ErrInvalidPoolTimeout.New()
	}

	select {
	case d, more := <-q.c:
		q.n.Add(-1)
		return d, more, nil
	case <-time.After(timeout):
		return nil, true, ErrPoolTimeout.New()
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
	return MovingAverage(10, MinMax(1, DefaultMaxInstancesPerDriver, AIMD(1, 0.5)))
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
