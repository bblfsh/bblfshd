// +build linux,cgo

package daemon

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/opentracing/opentracing-go"
	"gopkg.in/src-d/go-log.v1"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"github.com/bblfsh/bblfshd/v2/runtime"

	"github.com/bblfsh/sdk/v3/driver"
	"github.com/bblfsh/sdk/v3/driver/manifest"
	protocol2 "github.com/bblfsh/sdk/v3/protocol"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
)

const (
	// keepaliveMinTime is the minimum amount of time a client should wait before sending
	// a keepalive ping.
	keepaliveMinTime = 1 * time.Minute

	// keepalivePingWithoutStream is a boolean flag.
	// If true, server allows keepalive pings even when there are no active
	// streams(RPCs). If false, and client sends ping when there are no active
	// streams, server will send GOAWAY and close the connection.
	keepalivePingWithoutStream = true
)

// Daemon is a Babelfish server.
type Daemon struct {
	UserServer    *grpc.Server
	ControlServer *grpc.Server

	version   string
	build     time.Time
	runtime   *runtime.Runtime
	driverEnv []string

	mu      sync.RWMutex
	pool    map[string]*DriverPool // language ID → driver pool
	aliases map[string]string      // alias → language ID
}

// NewDaemon creates a new server based on the runtime with the given version.
func NewDaemon(version string, build time.Time, r *runtime.Runtime, opts ...grpc.ServerOption) *Daemon {
	commonOpt := append(protocol2.ServerOptions(),
		// EnforcementPolicy is used to set keepalive enforcement policy on the
		// server-side. Server will close connection with a client that violates this
		// policy.
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             keepaliveMinTime,
			PermitWithoutStream: keepalivePingWithoutStream,
		}),
	)
	opts = append(commonOpt, opts...)

	d := &Daemon{
		version:       version,
		build:         build,
		runtime:       r,
		pool:          make(map[string]*DriverPool),
		aliases:       make(map[string]string),
		UserServer:    grpc.NewServer(opts...),
		ControlServer: grpc.NewServer(commonOpt...),
	}
	registerGRPC(d)
	// pass tracing options to each driver
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "JAEGER_") {
			continue
		}
		const traceHost = "JAEGER_AGENT_HOST="
		if strings.HasPrefix(traceHost, "JAEGER_") {
			// drivers cannot use Docker DNS as bblfshd does,
			// so we need to remap an address manually
			if addr := os.Getenv("JAEGER_PORT_6831_UDP_ADDR"); addr != "" {
				env = traceHost + addr
			}
		}
		d.driverEnv = append(d.driverEnv, env)
	}
	return d
}

func registerGRPC(d *Daemon) {
	protocol1.DefaultService = NewService(d)
	protocol1.RegisterProtocolServiceServer(d.UserServer, protocol1.NewProtocolServiceServer())

	s2 := NewServiceV2(d)
	protocol2.RegisterDriverServer(d.UserServer, s2)
	protocol2.RegisterDriverHostServer(d.UserServer, s2)
	protocol.RegisterService(d.ControlServer, NewControlService(d))
}

func (d *Daemon) InstallDriver(language string, image string, update bool) error {
	driverInstallCalls.Add(1)

	img, err := runtime.NewDriverImage(image)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}
	if language == "" {
		info, err := img.Inspect()
		if err != nil {
			return err
		}
		if lang, ok := info.Labels["bblfsh.language"]; ok {
			language = lang
		} else {
			return ErrLanguageDetection.New()
		}
	}

	s, _, err := d.getDriverImage(context.TODO(), language)
	if err != nil && !driver.IsMissingDriver(err) {
		return ErrRuntime.Wrap(err)
	}
	if err == nil {
		if !update {
			return ErrAlreadyInstalled.Wrap(err, language, image)
		}
		// TODO: the old driver should be removed only after a successful install of the new one
		if err := d.runtime.RemoveDriver(s); err != nil {
			return err
		}
	}

	_, err = d.runtime.InstallDriver(img, update)
	if err != nil {
		return err
	}

	log.Infof("driver %s installed %q", language, img.Name())
	return nil
}

func (d *Daemon) RemoveDriver(language string) error {
	driverRemoveCalls.Add(1)

	img, _, err := d.getDriverImage(context.TODO(), language)
	if err != nil {
		return ErrRuntime.Wrap(err)
	}

	if err := d.runtime.RemoveDriver(img); err != nil {
		return err
	}
	if err := d.removePool(language); err != nil {
		return err
	}

	log.Infof("driver %s removed %q", language, img.Name())
	return err
}

func (d *Daemon) DriverPool(ctx context.Context, language string) (*DriverPool, error) {
	language = strings.ToLower(language)
	d.mu.RLock()
	if l, ok := d.aliases[language]; ok {
		language = l
	}
	dp, ok := d.pool[language]
	d.mu.RUnlock()
	if ok {
		return dp, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.aliases[language]; ok {
		language = l
	}
	dp, ok = d.pool[language]
	if ok {
		return dp, nil
	}

	image, m, err := d.getDriverImage(ctx, language)
	if err != nil {
		return nil, ErrRuntime.Wrap(err)
	}

	return d.newDriverPool(ctx, m.Language, m.Aliases, image)
}

func driverWithLang(lang string, list []*runtime.DriverImageStatus) *runtime.DriverImageStatus {
	lang = strings.ToLower(lang)
	for _, d := range list {
		m := d.Manifest
		if strings.ToLower(m.Language) == lang {
			return d
		}
		for _, l := range m.Aliases {
			if strings.ToLower(l) == lang {
				return d
			}
		}
	}
	return nil
}

func (d *Daemon) getDriverImage(rctx context.Context, language string) (runtime.DriverImage, *manifest.Manifest, error) {
	sp, _ := opentracing.StartSpanFromContext(rctx, "bblfshd.runtime.ListDrivers")
	defer sp.Finish()

	list, err := d.runtime.ListDrivers()
	if err != nil {
		return nil, nil, err
	}
	dr := driverWithLang(language, list)
	if dr == nil {
		return nil, nil, &ErrMissingDriver{language}
	}
	img, err := runtime.NewDriverImage(dr.Reference)
	return img, dr.Manifest, err
}

// newDriverPool, instance a new driver pool for the given language and image
// and should be called under a lock.
func (d *Daemon) newDriverPool(rctx context.Context, language string, aliases []string, image runtime.DriverImage) (*DriverPool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.pool.newDriverPool")
	defer sp.Finish()

	imageName := image.Name()
	labels := []string{language, imageName}

	dp := NewDriverPool(func(rctx context.Context) (Driver, error) {
		sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.pool.driverFactory")
		defer sp.Finish()

		log.Debugf("spawning driver instance %q ...", imageName)

		opts := d.getDriverInstanceOptions()
		driver, err := NewDriverInstance(d.runtime, language, image, opts)
		if err != nil {
			return nil, err
		}

		if err := driver.Start(ctx); err != nil {
			return nil, err
		}

		log.Infof("new driver instance started %s (%s)", imageName, driver.Container.ID())
		return driver, nil
	})
	dp.SetLabels(labels)

	if err := dp.Start(ctx); err != nil {
		return nil, err
	}

	d.pool[language] = dp
	for _, l := range aliases {
		log.Debugf("language alias: %s = %s", language, l)
		d.aliases[strings.ToLower(l)] = language
	}
	return dp, nil
}

func (d *Daemon) removePool(language string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	dp, ok := d.pool[language]
	if !ok {
		return nil
	}
	if err := dp.Stop(); err != nil && !ErrPoolClosed.Is(err) {
		return err
	}
	delete(d.pool, language)
	return nil
}

// Current returns the current list of driver pools.
func (d *Daemon) Current() map[string]*DriverPool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make(map[string]*DriverPool, len(d.pool))
	for k, pool := range d.pool {
		out[k] = pool
	}

	return out
}

// Stop stops all the pools and containers.
func (d *Daemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var err error
	for _, dp := range d.pool {
		if cerr := dp.Stop(); cerr != nil && err != nil {
			err = cerr
		}
	}

	return err
}

func (d *Daemon) getDriverInstanceOptions() *Options {
	opts := &Options{Env: d.driverEnv}
	opts.LogLevel = log.DefaultLevel
	opts.LogFormat = "text"

	if log.DefaultFormat == log.JSONFormat {
		opts.LogFormat = "json"
	}

	return opts
}

func newResponseFromError(err error) protocol1.Response {
	return protocol1.Response{
		Status: protocol1.Fatal,
		Errors: []string{err.Error()},
	}
}
