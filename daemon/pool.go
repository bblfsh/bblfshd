// +build linux,cgo

package daemon

import (
	"context"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	"gopkg.in/src-d/go-errors.v1"
)

var (
	// DefaultMaxInstancesPerDriver is the maximum number of instances of
	// the same driver which can be launched following the default
	// scaling policy (see DefaultScalingPolicy()).
	DefaultMaxInstancesPerDriver = runtime.NumCPU()

	// ErrPoolClosed is returned if the pool was already closed or is being closed.
	ErrPoolClosed = errors.NewKind("driver pool already closed")
	// ErrPoolRunning is returned if the pool was already running.
	ErrPoolRunning = errors.NewKind("driver pool already running")

	errDriverStopped = errors.NewKind("driver stopped")
)

// DriverPool controls a pool of drivers and balances requests among them,
// ensuring each driver does not get concurrent requests. The number of driver
// instances in the driver pool is controlled by a ScalingPolicy.
type DriverPool struct {
	// ScalingPolicy scaling policy used to scale up the instances.
	ScalingPolicy ScalingPolicy
	// Logger used during the live of the driver pool.
	Logger server.Logger

	// factory function used to spawn new driver instances.
	factory FactoryFunction

	// wg tracks all goroutines owned by the driver pool.
	wg sync.WaitGroup

	// stop channel should be closed to send a stop signal to the pool.
	stop chan struct{}
	// stopped is closed when the pool (and the manager goroutine) stops.
	stopped chan struct{}

	// get serves as a request-response channel to get an idle driver instance.
	// This channel is used by clients and is accepted by the manager goroutine.
	get chan driverRequest
	// put returns the driver to the pool. The driver must be active.
	put chan Driver

	// rescale accepts signals passed from the runPolicy goroutine to the manager goroutine.
	// It allows to re-evaluate scaling conditions when waiting for an idle driver.
	rescale chan struct{}

	// spawn accepts signals to the spawner goroutine to run a new driver.
	// The driver is returned on the put channel.
	spawn chan struct{}

	// spawnErr optionally communicates the driver creation failures to client goroutines.
	// The spawn goroutine won't block waiting for this channel to accept an error and will
	// log the error instead, if no client goroutines are willing to wait for it.
	spawnErr chan error

	drivers struct {
		sync.RWMutex
		idle map[Driver]struct{}
		all  map[Driver]struct{}
	}

	requests   atomicInt // requests waiting for a driver
	running    atomicInt // total running instances; synced with len(drivers.all)
	spawning   atomicInt // instances being started
	targetSize atomicInt // instances wanted

	exited  atomicInt // drivers exited
	success atomicInt // requests executed successfully
	errors  atomicInt // requests failed
}

type driverRequest struct {
	// cancel channel is closes then the client request is cancelled. Set to ctx.Done().
	cancel <-chan struct{}
	// out receives a single Driver value to the client. Channel may also be closed,
	// signalling that the pool is closing. Either out or err will be triggered.
	out chan<- Driver
	// err receives a single error value in case the pool cannot retrieve or create
	// a driver instance. Either out or err will be triggered.
	err chan<- error
}

// FactoryFunction is a factory function that creates new DriverInstance's.
type FactoryFunction func(ctx context.Context) (Driver, error)

// NewDriverPool creates and starts a new DriverPool. It takes as parameters
// a FactoryFunction, used to instantiate new drivers.
func NewDriverPool(factory FactoryFunction) *DriverPool {
	dp := &DriverPool{
		ScalingPolicy: DefaultScalingPolicy(),
		Logger:        logrus.New(),
		factory:       factory,
	}
	return dp
}

// Start stats the driver pool.
func (dp *DriverPool) Start(ctx context.Context) error {
	if dp.stop != nil {
		return ErrPoolRunning.New()
	}
	select {
	case <-dp.stopped:
		return ErrPoolClosed.New()
	default:
	}

	dp.stop = make(chan struct{})
	dp.stopped = make(chan struct{})
	dp.spawn = make(chan struct{})
	dp.spawnErr = make(chan error)
	dp.rescale = make(chan struct{}, 1)
	dp.get = make(chan driverRequest)
	dp.put = make(chan Driver)
	dp.drivers.idle = make(map[Driver]struct{})
	dp.drivers.all = make(map[Driver]struct{})

	dp.targetSize.Set(1)

	dp.wg.Add(3)
	go func() {
		defer dp.wg.Done()
		dp.runSpawn()
	}()
	go func() {
		defer dp.wg.Done()
		dp.runPolicy()
	}()
	go func() {
		defer close(dp.stopped)
		defer dp.wg.Done()
		dp.manageDrivers()
	}()

	// wait for a single instance to come up
	d, err := dp.getDriver(ctx)
	if err != nil {
		_ = dp.Stop()
		return err
	}
	if err := dp.putDriver(d); err != nil {
		return err
	}
	return nil
}

// runPolicy goroutine re-evaluates the scaling policy on a regular time interval and sets
// a target number of instances. The scaling itself will be performed by the manager goroutine.
func (dp *DriverPool) runPolicy() {
	ticker := time.NewTicker(time.Millisecond * 100)
	defer ticker.Stop()
	for {
		select {
		case <-dp.stop:
			return
		case <-ticker.C:
		}
		total := dp.running.Value()
		load := dp.requests.Value()
		dp.drivers.RLock()
		idle := len(dp.drivers.idle)
		dp.drivers.RUnlock()

		target := dp.ScalingPolicy.Scale(total, load-idle)
		if target < 1 {
			target = 1 // there should be always at least 1 instance
		}
		old := dp.targetSize.Set(target)
		if old != target {
			// optionally signal to the manager goroutine
			select {
			case dp.rescale <- struct{}{}:
			default:
			}
		}
	}
}

// spawnOne starts a new driver instance. It will keep trying to run it in case of a failure.
func (dp *DriverPool) spawnOne() {
	dp.spawning.Add(1)
	defer dp.spawning.Add(-1)

	// TODO(dennwc): use exponential backoff instead?
	ticker := time.NewTicker(time.Millisecond * 250)
	defer ticker.Stop()

	// keep trying in case of a failure
	for {
		d, err := dp.factory(chanContext(dp.stop))
		if err == nil {
			dp.drivers.Lock()
			dp.drivers.all[d] = struct{}{}
			dp.running.Add(1)
			dp.drivers.Unlock()

			err = dp.putDriver(d)
			if err == nil {
				return // done
			}
		}
		dp.Logger.Errorf("failed to start a driver: %v", err)
		select {
		case <-dp.stop:
			return // cancel
		case dp.spawnErr <- err:
		case <-ticker.C:
		}
	}
}

// runSpawn is a goroutine responsible for spawning new instances in the background.
func (dp *DriverPool) runSpawn() {
	for {
		select {
		case <-dp.stop:
			return
		case <-dp.spawn:
			dp.spawnOne()
		}
	}
}

// peekIdle tries to get an idle driver from the pool. It won't wait for the driver to
// become idle, instead it will return false if there are no idle drivers.
func (dp *DriverPool) peekIdle() (Driver, bool) {
	dp.drivers.RLock()
	n := len(dp.drivers.idle)
	dp.drivers.RUnlock()
	if n == 0 {
		return nil, false
	}
	dp.drivers.Lock()
	defer dp.drivers.Unlock()
	for d := range dp.drivers.idle {
		delete(dp.drivers.idle, d)
		return d, true
	}
	return nil, false
}

// setIdle returns the driver to an idle state.
func (dp *DriverPool) setIdle(d Driver) {
	dp.drivers.Lock()
	defer dp.drivers.Unlock()
	dp.drivers.idle[d] = struct{}{}
}

// killDriver stops are removes the driver from the queue.
func (dp *DriverPool) killDriver(d Driver) {
	dp.drivers.Lock()
	delete(dp.drivers.all, d)
	delete(dp.drivers.idle, d)
	dp.running.Add(-1)
	dp.exited.Add(1)
	dp.drivers.Unlock()

	if err := d.Stop(); err != nil {
		dp.Logger.Errorf("error removing stopped driver: %s", err)
	}
}

// scaleDiff returns current difference between the target number of instances and the
// current number of running instances. This is positive when scaling up, and negative
// when scaling down.
func (dp *DriverPool) scaleDiff() int {
	total := dp.running.Value()
	return dp.targetSize.Value() - total
}

// rescaleLater interrupts the scaling and serves the request first.
// It will make sure to continue scaling later.
func (dp *DriverPool) rescaleLater(req driverRequest) {
	select {
	case dp.rescale <- struct{}{}:
	default:
	}
	dp.waitOrScale(req)
}

// scale the driver pool to the current target number of instances.
func (dp *DriverPool) scale() {
	dn := dp.scaleDiff()
	if dn == 0 {
		return
	}

	if dn < 0 {
		// scale down
		for i := 0; i < -dn; i++ {
			if d, ok := dp.peekIdle(); ok {
				dp.killDriver(d)
				continue
			}
			select {
			case <-dp.stop:
				return
			case req := <-dp.get:
				dp.rescaleLater(req)
				return
			case d := <-dp.put:
				dp.killDriver(d)
			}
		}
		return
	}
	// scale up
	for i := 0; i < dn; i++ {
		select {
		case req := <-dp.get:
			dp.rescaleLater(req)
			return
		case <-dp.stop:
			return
		case d := <-dp.put:
			// received some existing instance
			dp.setIdle(d)
			i--
			dn = dp.scaleDiff()
		case dp.spawn <- struct{}{}:
			// spawn request sent to the spawn goroutine
			select {
			case <-dp.stop:
				return
			case d := <-dp.put:
				dp.setIdle(d)
			case <-dp.spawnErr:
				// ignore - already printed to the log
			}
		}
	}
}

// returnDriver will either return the driver to the client,
// or will return it to the idle driver queue.
func (dp *DriverPool) returnDriver(req driverRequest, d Driver) {
	select {
	case <-req.cancel:
		dp.setIdle(d)
	case req.out <- d:
	}
}

// scaleUp serves the user request, assuming that the pool is allowed to scale up.
func (dp *DriverPool) scaleUp(req driverRequest) {
	select {
	case d := <-dp.put:
		dp.returnDriver(req, d)
	case err := <-dp.spawnErr:
		req.err <- err
	case dp.spawn <- struct{}{}:
		// spawn request sent to the spawn goroutine
		select {
		case d := <-dp.put:
			dp.returnDriver(req, d)
		case err := <-dp.spawnErr:
			req.err <- err
		case <-req.cancel:
		}
	case <-req.cancel:
	}
}

// scaleDown serves the user request, assuming that the pool is scaling down, or not
// allowed to scale up anymore. It returns the flag whether the request was fulfilled.
func (dp *DriverPool) scaleDown(req driverRequest, exact bool) bool {
	select {
	case <-req.cancel:
		return true
	case err := <-dp.spawnErr:
		req.err <- err
		return true
	case d := <-dp.put:
		if exact {
			// exactly the right amount of instances
			dp.returnDriver(req, d)
			return true
		}
		// bad luck - we are scaling down
		// TODO(dennwc): add some metric to track if there are cases when the
		//               scaling policy asks us to drain and then asks to scale
		//               back up - we could have returned this driver to the
		//               client instead
		dp.killDriver(d)
	case <-dp.rescale:
		// worth to re-evaluate scaling conditions
	}
	return false
}

// waitOrScale is executed on the manager goroutine. It will either scale the pool up,
// wait for an instance to become available, or scale the pool down. The function will
// block until the request is served or cancelled.
func (dp *DriverPool) waitOrScale(req driverRequest) {
	// Note that we don't care about the pool closing in this function.
	// If it happens, the client will receive this signal before we do and will
	// cancel the request anyway. This reduces the number of select statements.

	// loop allows to re-evaluate scaling conditions
	for {
		dn := dp.scaleDiff()

		if dp.running.Value()+dp.spawning.Value() == 0 {
			if dn < 0 {
				// This shouldn't really happen: pool is draining, but there are no running
				// instances. In any case, we don't want to deadlock here, so we will at least
				// cancel the request with an error.
				dp.Logger.Warningf("cannot serve the request: pool is draining")
				req.err <- ErrPoolClosed.New()
				return
			} else if dn == 0 {
				// No instances running and there are running requests in the background,
				// but the policy doesn't allow us to scale up.
				//
				// This may happen when there were no requests for a long time, and the
				// policy is not smart enough to allow us to run even a single instance.
				//
				// So we will pretend we are allowed to run one.
				dn = 1
			}
			// dn > 0
		}
		if d, ok := dp.peekIdle(); ok {
			dp.returnDriver(req, d)
			return
		}

		if dn > 0 {
			// allowed to scale up
			dp.scaleUp(req)
			return
		}
		// dn <= 0

		// not allowed to scale up or we are scaling down
		if dp.scaleDown(req, dn == 0) {
			return
		}
	}
}

// drain waits until all instances are stopped. Should only be called from the
// manager goroutine. It assumes the stop channel already triggered.
func (dp *DriverPool) drain() {
	defer dp.targetSize.Set(0)
	for {
		d, ok := dp.peekIdle()
		if !ok {
			break
		}
		dp.killDriver(d)
	}
	for dp.running.Value() > 0 {
		d := <-dp.put
		dp.killDriver(d)
	}
}

// manageDrivers is the main goroutine responsible for managing drivers.
// It will accept all client requests for drivers if there are no drivers in the idle state.
// It will also take care of draining instances when the pool closes.
func (dp *DriverPool) manageDrivers() {
	defer dp.drain()

	for {
		select {
		case d := <-dp.put:
			dp.setIdle(d)
		case req := <-dp.get:
			dp.waitOrScale(req)
		case <-dp.rescale:
			dp.scale()
		case <-dp.stop:
			return
		}
	}
}

// getIdle returns an idle driver from the queue. It won't check the driver status.
// After the driver is returned, it's owned by the caller, but it still counts toward the
// pool scaling limit. The caller should put the instance back to the pool even if the
// driver fails.
func (dp *DriverPool) getIdle(rctx context.Context) (Driver, error) {
	// fast path - get an idle driver directly from the pool
	// this function executes on the current goroutine
	if d, ok := dp.peekIdle(); ok {
		return d, nil
	}

	// slow path - ask the manager goroutine to pick an instance for us
	dp.requests.Add(1)
	defer dp.requests.Add(-1)

	// ensure we can cancel our request on return
	ctx, cancel := context.WithCancel(rctx)
	defer cancel()

	resp := make(chan Driver)
	errc := make(chan error, 1)
	req := driverRequest{
		out: resp, err: errc, cancel: ctx.Done(),
	}
	select {
	case <-req.cancel: // parent context cancelled
		return nil, ctx.Err()
	case err := <-errc:
		return nil, err
	case dp.get <- req: // send request to get a driver
		select {
		case <-req.cancel:
			return nil, ctx.Err()
		case err := <-errc:
			return nil, err
		case d, ok := <-resp:
			if ok {
				return d, nil
			}
		case <-dp.stop:
		}
	case <-dp.stop:
	}
	return nil, ErrPoolClosed.New()
}

// putDriver returns the driver to the pool.
func (dp *DriverPool) putDriver(d Driver) error {
	if err := dp.checkStatus(d); err != nil {
		return err
	}
	select {
	case <-dp.stop:
		dp.killDriver(d)
		return ErrPoolClosed.New()
	case dp.put <- d:
	}
	return nil
}

// checkStatus will check if driver is still active. If not, the function returns an error
// and removes the driver from the pool.
func (dp *DriverPool) checkStatus(d Driver) error {
	status, err := d.Status()
	if err != nil {
		dp.Logger.Errorf("error getting driver status, removing: %s", err)
		dp.killDriver(d)
		return err
	} else if status != protocol.Running {
		dp.Logger.Debugf("removing stopped driver")
		dp.killDriver(d)
		return errDriverStopped.New()
	}
	return nil
}

// FunctionCtx is a function to be executed using a given driver.
type FunctionCtx func(ctx context.Context, d Driver) error

// Execute executes the given Function in the first available driver instance.
// It gets a driver from the pool and forwards the request to it. If all drivers
// are busy, it will return an error after the timeout passes. If the DriverPool
// is closed, an error will be returned.
//
// Deprecated: use ExecuteCtx instead.
func (dp *DriverPool) Execute(c FunctionCtx, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return dp.ExecuteCtx(ctx, c)
}

// ExecuteCtx executes the given Function in the first available driver instance.
// It gets a driver from the pool and forwards the request to it. If all drivers
// are busy, it will return an error after the timeout passes. If the DriverPool
// is closed, an error will be returned.
func (dp *DriverPool) ExecuteCtx(rctx context.Context, c FunctionCtx) error {
	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.pool.Execute")
	defer sp.Finish()

	d, err := dp.getDriver(ctx)
	if err != nil {
		dp.errors.Add(1)
		return err
	}
	defer dp.putDriver(d)

	if err := c(ctx, d); err != nil {
		dp.errors.Add(1)
		return err
	}

	dp.success.Add(1)
	return nil
}

// getDriver returns an idle driver instance. It will ensure that driver is running.
func (dp *DriverPool) getDriver(rctx context.Context) (Driver, error) {
	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.pool.getDriver")
	defer sp.Finish()

	for {
		d, err := dp.getIdle(ctx)
		if ErrPoolClosed.Is(err) {
			return nil, err
		} else if err != nil {
			dp.Logger.Warningf("unable to allocate a driver instance: %s", err)
			return nil, err
		}
		if dp.checkStatus(d) == nil {
			return d, nil
		}
		// retry until the deadline
	}
}

// Current returns a list of the current instances from the pool, it includes
// the running ones and those being stopped.
func (dp *DriverPool) Current() []Driver {
	dp.drivers.RLock()
	defer dp.drivers.RUnlock()
	list := make([]Driver, 0, len(dp.drivers.all))
	for d := range dp.drivers.all {
		list = append(list, d)
	}
	return list
}

// State current state of driver pool.
func (dp *DriverPool) State() *protocol.DriverPoolState {
	return &protocol.DriverPoolState{
		Wanted:  dp.targetSize.Value(),
		Running: dp.running.Value(),
		Waiting: dp.requests.Value(),
		Success: dp.success.Value(),
		Errors:  dp.errors.Value(),
		Exited:  dp.exited.Value(),
	}
}

// Stop stop the driver pool, including all its underlying driver instances.
func (dp *DriverPool) Stop() error {
	if dp.stop == nil {
		return nil // not running
	}
	select {
	case <-dp.stop:
		<-dp.stopped
		return ErrPoolClosed.New()
	case <-dp.stopped:
		return ErrPoolClosed.New()
	default:
		close(dp.stop)
		dp.wg.Wait()
		<-dp.stopped
		return nil
	}
}

type atomicInt struct {
	val int32
}

func (c *atomicInt) Set(n int) int {
	return int(atomic.SwapInt32(&c.val, int32(n)))
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
	sub   ScalingPolicy // policy that will use an average
	loads []float64
	pos   int
}

// MovingAverage computes a moving average of the load and forwards it to the
// underlying scaling policy. This policy is stateful and not thread-safe, do not
// reuse its instances for multiple pools.
func MovingAverage(window int, p ScalingPolicy) ScalingPolicy {
	return &movingAverage{
		sub:   p,
		loads: make([]float64, 0, window),
		pos:   0,
	}
}

func (p *movingAverage) Scale(total, load int) int {
	if len(p.loads) < cap(p.loads) {
		p.loads = append(p.loads, float64(load))
	} else {
		p.loads[p.pos] = float64(load)
	}
	p.pos++
	if p.pos >= cap(p.loads) {
		p.pos = 0
	}

	var sum float64
	for _, v := range p.loads {
		sum += v
	}

	avg := sum / float64(len(p.loads))
	return p.sub.Scale(total, int(avg))
}

type minMax struct {
	sub      ScalingPolicy // policy to take min-max from
	min, max int
}

// MinMax wraps a ScalingPolicy and applies a minimum and maximum to the number
// of instances.
func MinMax(min, max int, p ScalingPolicy) ScalingPolicy {
	return &minMax{
		sub: p,
		min: min,
		max: max,
	}
}

func (p *minMax) Scale(total, load int) int {
	s := p.sub.Scale(total, load)
	if s > p.max {
		return p.max
	}

	if s < p.min {
		return p.min
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

// chanContext wraps a receive-only channel to act as a context.
type chanContext <-chan struct{}

// Deadline implements context.Context.
func (c chanContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

// Done implements context.Context.
func (c chanContext) Done() <-chan struct{} {
	return c
}

// Err implements context.Context.
func (c chanContext) Err() error {
	select {
	case <-c:
		return context.Canceled
	default:
	}
	return nil
}

// Value implements context.Context.
func (c chanContext) Value(key interface{}) interface{} {
	return nil
}
