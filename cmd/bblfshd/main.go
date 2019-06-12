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

	"github.com/bblfsh/bblfshd/daemon"
	"github.com/bblfsh/bblfshd/runtime"

	cmdutil "github.com/bblfsh/sdk/v3/cmd"
	"github.com/bblfsh/sdk/v3/driver/manifest/discovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	pversion "github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
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
	log struct {
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

	log.level = cmd.String("log-level", "info", "log level: panic, fatal, error, warning, info, debug.")
	log.format = cmd.String("log-format", "text", "format of the logs: text or json.")
	log.fields = cmd.String("log-fields", "", "extra fields to add to every log line in json format.")
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
		logrus.Infof("installing driver for %s (%s)", dr.Language, image)
		err = d.InstallDriver(dr.Language, image, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	logrus.Infof("bblfshd version: %s (build: %s)", version, build)

	if *pprof.enabled {
		logrus.Infof("running pprof on %s", *pprof.address)
		go func() {
			if err := http.ListenAndServe(*pprof.address, nil); err != nil {
				logrus.Errorf("cannot start pprof: %v", err)
			}
		}()
	}
	if *metrics.address != "" {
		logrus.Infof("running metrics on %s", *metrics.address)
		go func() {
			if err := http.ListenAndServe(*metrics.address, promhttp.Handler()); err != nil {
				logrus.Errorf("cannot start metrics: %v", err)
			}
		}()
	}

	if os.Getenv("JAEGER_AGENT_HOST") != "" {
		c, err := jaegercfg.FromEnv()
		if err != nil {
			logrus.Fatalf("error configuring tracer: %s", err)
		}
		closer, err := c.InitGlobalTracer("bblfshd")
		if err != nil {
			logrus.Fatalf("error configuring tracer: %s", err)
		}
		defer closer.Close()
	}

	r := buildRuntime()
	grpcOpts, err := cmdutil.GRPCSizeOptions(*maxMessageSize)
	if err != nil {
		logrus.Fatalln(err)
	}

	parsedBuild, err := time.Parse(buildDateFormat, build)
	if err != nil {
		if build == undefined {
			parsedBuild = time.Now()
			logrus.Infof("using start time instead in this dev build: %s",
				parsedBuild.Format(buildDateFormat))
		} else {
			logrus.Fatalf("wrong date format for this build: %s", err)
		}
	}
	d := daemon.NewDaemon(version, parsedBuild, r, grpcOpts...)
	if args := cmd.Args(); len(args) == 2 && args[0] == "install" && args[1] == "recommended" {
		err := installRecommended(d)
		if err != nil {
			logrus.Fatalf("error listing drivers: %s", err)
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
		logrus.Fatalf("error creating listener: %s", err)
	}

	allowAnyoneInUnixSocket(*network, *address)
	logrus.Infof("server listening in %s (%s)", *address, *network)
	if err = d.UserServer.Serve(usrListener); err != nil {
		logrus.Fatalf("error starting server: %s", err)
	}
}

func listenControl(d *daemon.Daemon) {
	var err error
	if *ctl.network == "unix" {
		// Remove returns an error if file does not exists
		// if it returns nil, we know the file existed, so bblfshd might have crashed
		if err := os.Remove(*ctl.address); err == nil {
			logrus.Warningf("control socket %s (%s) already exists", *ctl.address, *ctl.network)
		}
	}
	ctlListener, err = net.Listen(*ctl.network, *ctl.address)
	if err != nil {
		logrus.Fatalf("error creating control listener: %s", err)
	}

	allowAnyoneInUnixSocket(*ctl.network, *ctl.address)
	logrus.Infof("control server listening in %s (%s)", *ctl.address, *ctl.network)
	if err = d.ControlServer.Serve(ctlListener); err != nil {
		logrus.Fatalf("error starting control server: %s", err)
	}
}

func allowAnyoneInUnixSocket(network, address string) {
	if network != "unix" {
		return
	}

	if err := os.Chmod(address, 0777); err != nil {
		logrus.Fatalf("error changing permissions to socket %q: %s", address, err)
	}
}

func buildLogger() {
	err := server.LoggerFactory{
		Level:  *log.level,
		Format: *log.format,
		Fields: *log.fields,
	}.Apply()

	if err != nil {
		logrus.Errorf("invalid logger configuration: %s", err)
		os.Exit(1)
	}
}

func buildRuntime() *runtime.Runtime {
	logrus.Infof("initializing runtime at %s", *storage)

	r := runtime.NewRuntime(*storage)
	if err := r.Init(); err != nil {
		logrus.Fatalf("error initializing runtime: %s", err)
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
	logrus.Warningf("signal received %+v", sig)
	logrus.Warningf("stopping server")
	if err := d.Stop(); err != nil {
		logrus.Errorf("error stopping server: %s", err)
	}

	for _, l := range []net.Listener{ctlListener, usrListener} {
		if err := l.Close(); err != nil {
			logrus.Errorf("error closing listener: %s", err)
		}
	}

	os.Exit(0)
}
