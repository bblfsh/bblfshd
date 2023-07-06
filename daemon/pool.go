// +build linux,cgo

package daemon

import (
	"context"
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/src-d/go-log.v1"

	"github.com/cenkalti/backoff"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"gopkg.in/src-d/go-errors.v1"
)

var (
	// DefaultMaxInstancesPerDriver is the maximum number of instances of
	// the same driver which can be launched following the default
	// scaling policy (see DefaultScalingPolicy()).
	//
	// Can be changed by setting BBLFSHD_MAX_DRIVER_INSTANCES.
	DefaultMaxInstancesPerDriver = mustEnvInt("BBLFSHD_MAX_DRIVER_INSTANCES", runtime.NumCPU())
	// DefaultMinInstancesPerDriver is the minimal number of instances of
	// the same driver which will be launched following the default
	// scaling policy (see DefaultScalingPolicy()).
	//
	// Can be changed by setting BBLFSHD_MIN_DRIVER_INSTANCES.
	DefaultMinInstancesPerDriver = mustEnvInt("BBLFSHD_MIN_DRIVER_INSTANCES", 1)

	// ErrPoolClosed is returned if the pool was already closed or is being closed.
	ErrPoolClosed = errors.NewKind("driver pool already closed")
	// ErrPoolRunning is returned if the pool was already running.
	ErrPoolRunning = errors.NewKind("driver pool already running")

	errDriverStopped = errors.NewKind("driver stopped")
)

const defaultPolicyTargetWindow = 5 // enough to prevent flickering

var (
	// policyDefaultWindow is a window for the average function used in the default scaling
	// policy. The window will be divided by policyDefaultTick intervals to calculate the
	// size of the window buffer, so this should ideally be a multiple of policyDefaultTick.
	policyDefaultWindow = mustEnvDur("BBLFSHD_POLICY_WINDOW", 5*time.Second)
	// policyDefaultTick is a tick rate for the goroutine that re-evaluates the scaling
	// policy for each driver pool.
	policyDefaultTick = mustEnvDur("BBLFSHD_POLICY_TICK", 500*time.Millisecond)
	// policyDefaultScale is a default increment for an additive increase scaling.
	//
	// See AIMD for more details.
	policyDefaultScale = mustEnvInt("BBLFSHD_POLICY_SCALE_INC", 1)
	// policyDefaultDownscale is a default multiplier for a multiplicative decrease downscaling.
	//
	// See AIMD for more details.
	policyDefaultDownscale = mustEnvFloat("BBLFSHD_POLICY_DOWNSCALE_MULT", 0.25)
)

func mustEnvInt(env string, def int) int {
	s := os.Getenv(env)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return v
}

func mustEnvFloat(env string, def float64) float64 {
	s := os.Getenv(env)
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return v
}

func mustEnvDur(env string, def time.Duration) time.Duration {
	s := os.Getenv(env)
	if s == "" {
		return def
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return v
}

// DriverPool controls a pool of drivers and balances requests among them,
// ensuring each driver does not get concurrent requests. The number of driver
// instances in the driver pool is controlled by a ScalingPolicy.
type DriverPool struct {
	// ScalingPolicy scaling policy used to scale up the instances.
	ScalingPolicy ScalingPolicy
	// Logger used during the live of the driver pool.
	Logger log.Logger

	// factory function used to spawn new driver instances.
	factory FactoryFunction

	// wg tracks all goroutines owned by the driver pool.
	wg sync.WaitGroup

	// poolCtx will be cancelled as a signal that the pool is closing.
	poolCtx context.Context
	// stop is called to send a stop channel to the pool. stopped channel will be closed
	// when the pool is fully stopped.
	stop func()

	// stopped is closed when the pool (and the manager goroutine) stops.
	stopped chan struct{}

	// get serves as a request-response channel to get an idle driver instance.
	// This channel is used by clients and is accepted by the manager goroutine.
	get chan driverRequest
	// put returns the driver to the pool. The driver must be active.
	put chan Driver

	// rescale accepts signals passed from the runPolicy goroutine to the manager goroutine.
	// It allows to re-evaluate scaling conditions when waiting for an idle driver.
	// The channel must have a buffer and sends to this channel must be used with default.
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

	metrics struct {
		scaling struct {
			total  prometheus.Gauge
			idle   prometheus.Gauge
			load   prometheus.Gauge
			target prometheus.Gauge
		}
		spawn struct {
			total prometheus.Counter
			err   prometheus.Counter
			kill  prometheus.Counter
		}
	}
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
	return &DriverPool{
		ScalingPolicy: DefaultScalingPolicy(),
		Logger:        log.New(nil),
		factory:       factory,
	}
}

func (dp *DriverPool) SetLabels(labels []string) {
	dp.Logger = log.DefaultLogger.With(log.Fields{
		"language": labels[0],
		"image":    labels[1],
	})

	dp.metrics.spawn.total = driversSpawned.WithLabelValues(labels...)
	dp.metrics.spawn.err = driversSpawnErrors.WithLabelValues(labels...)
	dp.metrics.spawn.kill = driversKilled.WithLabelValues(labels...)

	dp.metrics.scaling.total = driversRunning.WithLabelValues(labels...)
	dp.metrics.scaling.idle = driversIdle.WithLabelValues(labels...)
	dp.metrics.scaling.load = driversRequests.WithLabelValues(labels...)
	dp.metrics.scaling.target = driversTarget.WithLabelValues(labels...)
}

// Start stats the driver pool.
func (dp *DriverPool) Start(ctx context.Context) error {
	if dp.poolCtx != nil {
		return ErrPoolRunning.New()
	}
	select {
	case <-dp.stopped:
		return ErrPoolClosed.New()
	default:
	}

	// Yes, it's discouraged to use a long-lived context.
	// But an alternative is to re-implement a root Context, which is even worse.
	dp.poolCtx, dp.stop = context.WithCancel(context.Background())

	// This channel is read by the pool manager goroutine as a signal to rescale.
	// The scaling policy goroutine write to this channel whenever the allow number of
	// drivers changes. The channel must have a buffer of at least 1 and sends to the
	// channel should not block.
	dp.rescale = make(chan struct{}, 1)

	dp.stopped = make(chan struct{})
	dp.spawn = make(chan struct{})
	dp.spawnErr = make(chan error)
	dp.get = make(chan driverRequest)
	dp.put = make(chan Driver)
	dp.drivers.idle = make(map[Driver]struct{})
	dp.drivers.all = make(map[Driver]struct{})

	dp.targetSize.Set(1)

	dp.wg.Add(3)
	go func() {
		defer dp.wg.Done()
		dp.runSpawn(dp.poolCtx)
	}()
	go func() {
		defer dp.wg.Done()
		dp.runPolicy(dp.poolCtx)
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
func (dp *DriverPool) runPolicy(ctx context.Context) {
	ticker := time.NewTicker(policyDefaultTick)
	defer ticker.Stop()

	stop := ctx.Done()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		dp.drivers.RLock()
		total := dp.running.Value()
		load := dp.requests.Value()
		idle := len(dp.drivers.idle)
		dp.drivers.RUnlock()

		target := dp.ScalingPolicy.Scale(total, idle, load)
		if target < 1 {
			// there should be always at least 1 instance
			// TODO(dennwc): policies must never return 0 instances
			target = 1
		}
		if dp.metrics.scaling.total != nil {
			dp.metrics.scaling.total.Set(float64(total))
			dp.metrics.scaling.load.Set(float64(load))
			dp.metrics.scaling.idle.Set(float64(idle))
			dp.metrics.scaling.target.Set(float64(target))
		}
		old := dp.targetSize.Set(target)
		if old != target {
			// send a signal to the manager goroutine
			select {
			// the channel has buffer of 1 so it acts like a deferred signal
			// the send will fail only if the channel is already full, meaning
			// that manager goroutine haven't had time to receive the previous
			// signal yet, and it's ignore the signal we are trying to send
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

	ticker := backoff.NewTicker(backoff.NewExponentialBackOff())
	defer ticker.Stop()

	// keep trying in case of a failure
	ctx := dp.poolCtx
	stop := ctx.Done()
	for {
		if dp.metrics.spawn.total != nil {
			dp.metrics.spawn.total.Add(1)
		}
		d, err := dp.factory(ctx)
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
		if dp.metrics.spawn.err != nil {
			dp.metrics.spawn.err.Add(1)
		}
		dp.Logger.Errorf(err, "failed to start a driver")
		select {
		case <-stop:
			return // cancel
		case dp.spawnErr <- err:
		case _, ok := <-ticker.C:
			if !ok {
				dp.Logger.Errorf(err, "driver keeps failing, closing the pool; error")
				// can only run in a goroutine, since Stop will wait for runSpawn to return
				go dp.Stop()
				return
			}
		}
	}
}

// runSpawn is a goroutine responsible for spawning new instances in the background.
func (dp *DriverPool) runSpawn(ctx context.Context) {
	stop := ctx.Done()
	for {
		select {
		case <-stop:
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
func (dp *DriverPool) killDriver(d Driver, info string, err error) {
	if dp.metrics.spawn.kill != nil {
		dp.metrics.spawn.kill.Add(1)
	}

	if err != nil {
		dp.Logger.Errorf(err, "killDriver(%s): %s", d.ID(), info)
	} else {
		dp.Logger.Infof("killDriver(%s): %s", d.ID(), info)
	}

	dp.drivers.Lock()
	delete(dp.drivers.all, d)
	delete(dp.drivers.idle, d)
	dp.running.Add(-1)
	dp.exited.Add(1)
	dp.drivers.Unlock()

	if err := d.Stop(); err != nil {
		dp.Logger.Errorf(err, "error removing stopped driver")
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

	stop := dp.poolCtx.Done()
	if dn < 0 {
		// scale down
		for i := 0; i < -dn; i++ {
			select {
			case <-stop:
				return
			case req := <-dp.get:
				// no idle drivers, and there is a client waiting for us
				// do the scaling "inline" while serving the request
				dp.rescaleLater(req)
				return
			case d := <-dp.put:
				// prefer to kill driver that are returned by clients instead an idle ones
				// idle map may be accessed without management goroutine, thus it's more
				// valuable to keep it full
				dp.killDriver(d, "scale down - kill driver returned by client", nil)
				continue
			default:
			}
			// only idle drivers remain - start killing those
			if d, ok := dp.peekIdle(); ok {
				dp.killDriver(d, "scale down - kill idle driver", nil)
				continue
			}
			// no drivers are idle, only way to downscale is to wait for clients to put
			// their drivers back to the pool
			select {
			case <-stop:
				return
			case req := <-dp.get:
				dp.rescaleLater(req)
				return
			case d := <-dp.put:
				dp.killDriver(d, "scale down - no drivers are idle", nil)
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
		case <-stop:
			return
		case d := <-dp.put:
			// received some existing instance
			dp.setIdle(d)
			i--
			dn = dp.scaleDiff()
		case dp.spawn <- struct{}{}:
			// spawn request sent to the spawn goroutine
			select {
			case <-stop:
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
		dp.killDriver(d, "scaleDown", nil)
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
		dp.killDriver(d, "drain-peekIdle", nil)
	}
	for dp.running.Value() > 0 {
		d := <-dp.put
		dp.killDriver(d, "drain-put", nil)
	}
}

// manageDrivers is the main goroutine responsible for managing drivers.
// It will accept all client requests for drivers if there are no drivers in the idle state.
// It will also take care of draining instances when the pool closes.
func (dp *DriverPool) manageDrivers() {
	defer dp.drain()

	stop := dp.poolCtx.Done()
	for {
		select {
		case d := <-dp.put:
			dp.setIdle(d)
		case req := <-dp.get:
			dp.waitOrScale(req)
		case <-dp.rescale:
			dp.scale()
		case <-stop:
			return
		}
	}
}

// getIdle returns an idle driver from the queue. It won't check the driver status.
// After the driver is returned, it's owned by the caller, but it still counts toward the
// pool scaling limit. The caller should put the instance back to the pool even if the
// driver fails.
func (dp *DriverPool) getIdle(rctx context.Context) (Driver, error) {
	// don't do anything if the request is already cancelled,
	// or we will have to "rollback" it later
	select {
	case <-rctx.Done():
		return nil, rctx.Err()
	default:
	}
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
	stop := dp.poolCtx.Done()
	select {
	case <-req.cancel: // parent context cancelled (same as rctx.Done())
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
		case <-stop:
		}
	case <-stop:
	}
	return nil, ErrPoolClosed.New()
}

// putDriver returns the driver to the pool.
func (dp *DriverPool) putDriver(d Driver) error {
	if err := dp.checkStatus(d); err != nil {
		return err
	}
	select {
	case <-dp.poolCtx.Done():
		dp.killDriver(d, "putDriver", dp.poolCtx.Err())
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
		dp.killDriver(d, "error getting driver status, removing", err)
		return err
	} else if status != protocol.Running {
		dp.killDriver(d, "removing stopped driver", nil)
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

	if dp.poolCtx == nil {
		// not running
		return nil, ErrPoolClosed.New()
	}

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
	if dp.poolCtx == nil {
		return nil // not running
	}
	select {
	case <-dp.poolCtx.Done():
		<-dp.stopped
		return ErrPoolClosed.New()
	case <-dp.stopped:
		return ErrPoolClosed.New()
	default:
		dp.stop()
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
	// Scale takes the total number of active instances, number idle instances and the
	// number of requests waiting to get a driver instance. Idle may not be zero even if
	// number of waiting requests is non-zero.
	// Scale returns the new target number of instances to keep running. This number must
	// not be less than 1.
	Scale(total, idle, waiting int) int
}

// defaultScalingPolicy is the same as DefaultScalingPolicy, but has no window.
func defaultScalingPolicy() ScalingPolicy {
	min := DefaultMinInstancesPerDriver
	if min <= 0 {
		min = DefaultMaxInstancesPerDriver
	}
	return MinMax(
		min, DefaultMaxInstancesPerDriver,
		AIMD(policyDefaultScale, policyDefaultDownscale),
	)
}

// DefaultScalingPolicy returns a new instance of the default scaling policy.
// Instances returned by this function should not be reused.
func DefaultScalingPolicy() ScalingPolicy {
	windowIn := int(policyDefaultWindow / policyDefaultTick)
	return TargetMovingAverage(defaultPolicyTargetWindow, MovingAverage(windowIn, defaultScalingPolicy()))
}

func newMovingAverage(window int) *movingAverage {
	return &movingAverage{samples: make([]int, 0, window)}
}

type movingAverage struct {
	samples []int
	next    int
	sum     int
}

func (m *movingAverage) AddSample(v int) int {
	if len(m.samples) < cap(m.samples) {
		m.sum += v
		m.samples = append(m.samples, v)
		m.next++
	} else {
		next := m.next % cap(m.samples)
		m.sum = (m.sum + v) - m.samples[next]
		m.samples[next] = v
		m.next = next + 1
	}
	return int(math.Ceil(float64(m.sum) / float64(len(m.samples))))
}

type loadMovingAverage struct {
	sub   ScalingPolicy // policy that will use an average
	loads *movingAverage
}

// MovingAverage computes a moving average of the load and forwards it to the
// underlying scaling policy. This policy is stateful and not thread-safe, do not
// reuse its instances for multiple pools.
func MovingAverage(window int, p ScalingPolicy) ScalingPolicy {
	return &loadMovingAverage{
		sub:   p,
		loads: newMovingAverage(window),
	}
}

func (p *loadMovingAverage) Scale(total, idle, load int) int {
	avg := p.loads.AddSample(load)
	return p.sub.Scale(total, idle, avg)
}

type targetMovingAverage struct {
	sub     ScalingPolicy // policy that we will average
	targets *movingAverage
}

// TargetMovingAverage computes a moving average of the target instance count.
// This policy is stateful and not thread-safe, do not reuse its instances for multiple pools.
func TargetMovingAverage(window int, p ScalingPolicy) ScalingPolicy {
	return &targetMovingAverage{
		sub:     p,
		targets: newMovingAverage(window),
	}
}

func (p *targetMovingAverage) Scale(total, idle, load int) int {
	target := p.sub.Scale(total, idle, load)
	return p.targets.AddSample(target)
}

type minMax struct {
	sub      ScalingPolicy // policy to take min-max from
	min, max int
}

// MinMax wraps a ScalingPolicy and applies a minimum and maximum to the number
// of instances.
func MinMax(min, max int, p ScalingPolicy) ScalingPolicy {
	if min < 1 {
		min = 1
	}
	return &minMax{
		sub: p,
		min: min,
		max: max,
	}
}

func (p *minMax) Scale(total, idle, load int) int {
	v := p.sub.Scale(total, idle, load)
	if v > p.max {
		return p.max
	}
	if v < p.min {
		return p.min
	}
	return v
}

type aimd struct {
	add int
	mul float64
}

// AIMD returns a ScalingPolicy of additive increase / multiplicative decrease.
// Increases are of min(add, load). Decreases are of (unused * mul).
func AIMD(add int, mul float64) ScalingPolicy {
	return &aimd{add: add, mul: mul}
}

func (p *aimd) Scale(total, idle, waiting int) int {
	load := waiting - idle
	if load >= 0 {
		dn := p.add
		if p.add > load {
			dn = load
		}
		total += dn
	} else {
		unused := -load
		total -= int(math.Ceil(float64(unused) * p.mul))
	}

	if total < 1 {
		total = 1 // must not return 0
	}
	return total
}
