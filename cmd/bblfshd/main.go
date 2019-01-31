package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bblfsh/bblfshd/daemon"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/sirupsen/logrus"
	jaegercfg "github.com/uber/jaeger-client-go/config"

	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	cmdutil "gopkg.in/bblfsh/sdk.v2/cmd"
	"gopkg.in/bblfsh/sdk.v2/driver/manifest/discovery"
)

var (
	version = "undefined"
	build   = "undefined"

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
	cmd *flag.FlagSet

	usrListener net.Listener
	ctlListener net.Listener
)

func init() {
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

	c, err := jaegercfg.FromEnv()
	if err != nil {
		logrus.Errorf("error configuring tracer: %s", err)
		os.Exit(1)
	}
	closer, err := c.InitGlobalTracer("bblfshd")
	if err != nil {
		logrus.Errorf("error configuring tracer: %s", err)
		os.Exit(1)
	}
	defer closer.Close()

	r := buildRuntime()
	grpcOpts, err := cmdutil.GRPCSizeOptions(*maxMessageSize)
	if err != nil {
		logrus.Errorln(err)
		os.Exit(1)
	}

	parsedBuild, err := time.Parse("2006-01-02T15:04:05-07:00", build)
	if err != nil {
		logrus.Errorf("wrong date format for build: %s", err)
		os.Exit(1)
	}
	d := daemon.NewDaemon(version, parsedBuild, r, grpcOpts...)
	if args := cmd.Args(); len(args) == 2 && args[0] == "install" && args[1] == "recommended" {
		err := installRecommended(d)
		if err != nil {
			logrus.Errorf("error listing drivers: %s", err)
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
		logrus.Errorf("error creating listener: %s", err)
		os.Exit(1)
	}

	allowAnyoneInUnixSocket(*network, *address)
	logrus.Infof("server listening in %s (%s)", *address, *network)
	if err = d.UserServer.Serve(usrListener); err != nil {
		logrus.Errorf("error starting server: %s", err)
		os.Exit(1)
	}
}

func listenControl(d *daemon.Daemon) {
	var err error
	ctlListener, err = net.Listen(*ctl.network, *ctl.address)
	if err != nil {
		logrus.Errorf("error creating control listener: %s", err)
		os.Exit(1)
	}

	allowAnyoneInUnixSocket(*ctl.network, *ctl.address)
	logrus.Infof("control server listening in %s (%s)", *ctl.address, *ctl.network)
	if err = d.ControlServer.Serve(ctlListener); err != nil {
		logrus.Errorf("error starting control server: %s", err)
		os.Exit(1)
	}
}

func allowAnyoneInUnixSocket(network, address string) {
	if network != "unix" {
		return
	}

	if err := os.Chmod(address, 0777); err != nil {
		logrus.Errorf("error changing permissions to socket %q: %s", address, err)
		os.Exit(1)
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
		logrus.Errorf("error initializing runtime: %s", err)
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
