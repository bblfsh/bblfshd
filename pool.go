package server

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

const (
	DefaultPoolMin               = 1
	DefaultPoolMax               = 5
	DefaultPoolTimeBeforeNew     = time.Second * 3
	DefaultPoolTimeBetweenSpawns = time.Second * 10
	DefaultPoolTimeBeforeClean   = time.Minute * 1
)

// DriverPool controls a pool of drivers and balances requests among them,
// ensuring each driver does not get concurrent requests.
type DriverPool struct {
	// Min
	Min               int
	Max               int
	TimeBeforeNew     time.Duration
	TimeBetweenSpawns time.Duration
	TimeBeforeClean   time.Duration
	New               func() (Driver, error)

	cur     int
	ch      chan Driver
	m       *sync.Mutex
	close   chan struct{}
	closed  bool
	limiter *rate.Limiter
	last    time.Time
}

func NewDriverPool(new func() (Driver, error)) *DriverPool {
	return &DriverPool{
		Min:               DefaultPoolMin,
		Max:               DefaultPoolMax,
		TimeBeforeNew:     DefaultPoolTimeBeforeNew,
		TimeBetweenSpawns: DefaultPoolTimeBetweenSpawns,
		TimeBeforeClean:   DefaultPoolTimeBeforeClean,
		New:               new,
	}
}

// Start starts the DriverPool and runs the minimum number of instances.
func (dp *DriverPool) Start() error {
	dp.ch = make(chan Driver)
	dp.m = &sync.Mutex{}
	dp.close = make(chan struct{})
	dp.limiter = rate.NewLimiter(rate.Every(dp.TimeBetweenSpawns), 1)
	dp.last = time.Now()
	go dp.gc()
	return dp.SetInstanceCount(dp.Min)
}

// SetInstanceCount changes the number of running driver instances. Instances
// will be started or stopped as necessary to satisfy the new instance count.
// It blocks until the all required instances are started or stopped.
func (dp *DriverPool) SetInstanceCount(target int) error {
	dp.m.Lock()
	defer dp.m.Unlock()

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
		d, err := dp.New()
		if err != nil {
			return err
		}

		dp.enqueue(d)
		dp.cur++
	}

	return nil
}

func (dp *DriverPool) del(n int) error {
	for i := 0; i < n; i++ {
		d := <-dp.ch
		dp.cur--
		if err := d.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (dp *DriverPool) enqueue(d Driver) {
	go func() { dp.ch <- d }()
}

func (dp *DriverPool) inc() error {
	dp.m.Lock()
	defer dp.m.Unlock()
	if dp.cur >= dp.Max {
		logrus.Debugf("cannot increment instances, already got max: %d", dp.Max)
		return nil
	}

	if !dp.limiter.Allow() {
		logrus.Debugf("cannot increment instances, rate limited")
		return nil
	}

	logrus.Debugf("incrementing instances")
	return dp.add(1)
}

func (dp *DriverPool) dec() error {
	dp.m.Lock()
	defer dp.m.Unlock()
	if dp.cur <= dp.Min {
		logrus.Debugf("cannot decrement instances, already got min: %d", dp.Min)
		return nil
	}

	logrus.Debugf("decrement instances")
	return dp.del(1)
}

func (dp *DriverPool) gc() {
	for {
		time.Sleep(time.Millisecond * 100)
		ellapsed := time.Now().Sub(dp.last)
		if ellapsed > dp.TimeBeforeClean {
			dp.dec()
			dp.last = time.Now()
		}
	}
}

func (dp *DriverPool) ParseUAST(req *protocol.ParseUASTRequest) *protocol.ParseUASTResponse {
	for {
		if dp.closed {
			return dp.parseUAST(nil, false, req)
		}

		select {
		case d, more := <-dp.ch:
			return dp.parseUAST(d, more, req)
		case <-time.After(dp.TimeBeforeNew):
			logrus.Debug("timeout, trying to increment instances")
			go dp.inc()
			continue
		}
	}
}

func (dp *DriverPool) parseUAST(d Driver, more bool, req *protocol.ParseUASTRequest) *protocol.ParseUASTResponse {
	if !more {
		return &protocol.ParseUASTResponse{
			Status: protocol.Fatal,
			Errors: []string{"driver pool already closed"},
		}
	}

	defer dp.enqueue(d)
	dp.last = time.Now()
	return d.ParseUAST(req)
}

func (dp *DriverPool) Close() error {
	if dp.closed {
		return errors.New("already closed")
	}

	if err := dp.SetInstanceCount(0); err != nil {
		return err
	}

	dp.closed = true
	close(dp.ch)
	return nil
}
