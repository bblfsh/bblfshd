package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "net/http/pprof"

	"gopkg.in/src-d/go-log.v1"

	"github.com/bblfsh/bblfshd/v2/daemon"
	"github.com/bblfsh/bblfshd/v2/runtime"

	cmdutil "github.com/bblfsh/sdk/v3/cmd"
	"github.com/bblfsh/sdk/v3/driver/manifest/discovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	pversion "github.com/prometheus/common/version"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

const (
	undefined       = "undefined"
	buildDateFormat = "2006-01-02T15:04:05-0700"
)

var (
	version = undefined
	build   = undefined

	network        *string
	address        *string
	storage        *string
	transport      *string
	maxMessageSize *int

	ctl struct {
		network *string
		address *string
	}

	logcfg struct {
		level  *string
		format *string
		fields *string
	}

	pprof struct {
		enabled *bool
		address *string
	}
	metrics struct {
		address *string
	}
	cmd *flag.FlagSet

	usrListener net.Listener
	ctlListener net.Listener
)

func init() {
	pversion.Version = version
	pversion.BuildDate = build
	prometheus.MustRegister(pversion.NewCollector("bblfshd"))

	cmd = flag.NewFlagSet("bblfshd", flag.ExitOnError)
	network = cmd.String("network", "tcp", "network type: tcp, tcp4, tcp6, unix or unixpacket.")
	address = cmd.String("address", "0.0.0.0:9432", "address to listen.")
	storage = cmd.String("storage", "/var/lib/bblfshd", "path where all the runtime information is stored.")
	transport = cmd.String("transport", "docker", "default transport to fetch driver images: docker or docker-daemon)")
	maxMessageSize = cmdutil.FlagMaxGRPCMsgSizeMB(cmd)

	ctl.network = cmd.String("ctl-network", "unix", "control server network type: tcp, tcp4, tcp6, unix or unixpacket.")
	ctl.address = cmd.String("ctl-address", "/var/run/bblfshctl.sock", "control server address to listen.")

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logcfg.level = cmd.String("log-level", logLevel, "log level: panic, fatal, error, warning, info, debug.")
	logcfg.format = cmd.String("log-format", "text", "format of the logs: text or json.")
	logcfg.fields = cmd.String("log-fields", "", "extra fields to add to every log line in json format.")

	pprof.enabled = cmd.Bool("profiler", false, "run profiler http endpoint (pprof).")
	pprof.address = cmd.String("profiler-address", ":6060", "profiler address to listen on.")
	metrics.address = cmd.String("metrics-address", ":2112", "metrics address to listen on.")
	cmd.Parse(os.Args[1:])

	buildLogger()
	runtime.Bootstrap()
}

func driverImage(id string) string {
	return fmt.Sprintf("docker://bblfsh/%s-driver:latest", id)
}

func installRecommended(d *daemon.Daemon) error {
	ctx := context.Background()
	list, err := discovery.OfficialDrivers(ctx, &discovery.Options{
		NoMaintainers: true,
	})
	if err != nil {
		return err
	}
	for _, dr := range list {
		if !dr.IsRecommended() {
			continue
		}
		image := driverImage(dr.Language)
		log.Infof("installing driver for %s (%s)", dr.Language, image)
		err = d.InstallDriver(dr.Language, image, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	log.Infof("bblfshd version: %s (build: %s)", version, build)

	if *pprof.enabled {
		log.Infof("running pprof on %s", *pprof.address)
		go func() {
			if err := http.ListenAndServe(*pprof.address, nil); err != nil {
				log.Errorf(err, "cannot start pprof")
			}
		}()
	}
	if *metrics.address != "" {
		log.Infof("running metrics on %s", *metrics.address)
		go func() {
			if err := http.ListenAndServe(*metrics.address, promhttp.Handler()); err != nil {
				log.Errorf(err, "cannot start metrics")
			}
		}()
	}

	if os.Getenv("JAEGER_AGENT_HOST") != "" {
		c, err := jaegercfg.FromEnv()
		if err != nil {
			log.Errorf(err, "error configuring tracer")
			os.Exit(1)
		}
		closer, err := c.InitGlobalTracer("bblfshd")
		if err != nil {
			log.Errorf(err, "error configuring tracer")
			os.Exit(1)
		}
		defer closer.Close()
	}

	r := buildRuntime()
	grpcOpts, err := cmdutil.GRPCSizeOptions(*maxMessageSize)
	if err != nil {
		log.Errorf(err, "cannot get gRPC server options\n")
		os.Exit(1)
	}

	parsedBuild, err := time.Parse(buildDateFormat, build)
	if err != nil {
		if build == undefined {
			parsedBuild = time.Now()
			log.Infof("using start time instead in this dev build: %s",
				parsedBuild.Format(buildDateFormat))
		} else {
			log.Errorf(err, "wrong date format for this build")
			os.Exit(1)
		}
	}
	d := daemon.NewDaemon(version, parsedBuild, r, grpcOpts...)
	if args := cmd.Args(); len(args) == 2 && args[0] == "install" && args[1] == "recommended" {
		err := installRecommended(d)
		if err != nil {
			log.Errorf(err, "error listing drivers")
			os.Exit(1)
		}
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		listenUser(d)
	}()
	go func() {
		defer wg.Done()
		listenControl(d)
	}()
	handleGracefullyShutdown(d)
	wg.Wait()
}

func listenUser(d *daemon.Daemon) {
	var err error
	usrListener, err = net.Listen(*network, *address)
	if err != nil {
		log.Errorf(err, "error creating listener")
		os.Exit(1)
	}

	allowAnyoneInUnixSocket(*network, *address)
	log.Infof("server listening in %s (%s)", *address, *network)
	if err = d.UserServer.Serve(usrListener); err != nil {
		log.Errorf(err, "error starting server")
		os.Exit(1)
	}
}

func listenControl(d *daemon.Daemon) {
	var err error
	if *ctl.network == "unix" {
		// Remove returns an error if file does not exists
		// if it returns nil, we know the file existed, so bblfshd might have crashed
		if err := os.Remove(*ctl.address); err == nil {
			log.Warningf("control socket %s (%s) already exists", *ctl.address, *ctl.network)
		}
	}
	ctlListener, err = net.Listen(*ctl.network, *ctl.address)
	if err != nil {
		log.Errorf(err, "error creating control listener")
		os.Exit(1)
	}

	allowAnyoneInUnixSocket(*ctl.network, *ctl.address)
	log.Infof("control server listening in %s (%s)", *ctl.address, *ctl.network)
	if err = d.ControlServer.Serve(ctlListener); err != nil {
		log.Errorf(err, "error starting control server")
		os.Exit(1)
	}
}

func allowAnyoneInUnixSocket(network, address string) {
	if network != "unix" {
		return
	}

	if err := os.Chmod(address, 0777); err != nil {
		log.Errorf(err, "error changing permissions to socket %q", address)
		os.Exit(1)
	}
}

func buildLogger() {
	log.New(nil)

	f := log.DefaultFactory
	f.Level = *logcfg.level
	f.Format = *logcfg.format
	f.Fields = *logcfg.fields
	if err := f.ApplyToLogrus(); err != nil {
		log.Errorf(err, "invalid logger configuration")
		os.Exit(1)
	}
}

func buildRuntime() *runtime.Runtime {
	log.Infof("initializing runtime at %s", *storage)

	r := runtime.NewRuntime(*storage)
	if err := r.Init(); err != nil {
		log.Errorf(err, "error initializing runtime")
		os.Exit(1)
	}

	return r
}

func handleGracefullyShutdown(d *daemon.Daemon) {
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go waitForStop(gracefulStop, d)
}

func waitForStop(ch <-chan os.Signal, d *daemon.Daemon) {
	sig := <-ch
	log.Warningf("signal received %+v", sig)
	log.Warningf("stopping server")
	if err := d.Stop(); err != nil {
		log.Errorf(err, "error stopping server")
	}

	for _, l := range []net.Listener{ctlListener, usrListener} {
		if err := l.Close(); err != nil {
			log.Errorf(err, "error closing listener")
		}
	}

	os.Exit(0)
}
