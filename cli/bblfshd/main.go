package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/bblfsh/bblfshd/daemon"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
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

	usrListener net.Listener
	ctlListener net.Listener
	wg          sync.WaitGroup
)

func init() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	network = cmd.String("network", "tcp", "network type: tcp, tcp4, tcp6, unix or unixpacket.")
	address = cmd.String("address", "0.0.0.0:9432", "address to listen.")
	storage = cmd.String("storage", "/var/lib/bblfshd", "path where all the runtime information is stored.")
	transport = cmd.String("transport", "docker", "default transport to fetch driver images: docker or docker-daemon)")
	maxMessageSize = cmd.Int("grpc-max-message-size", 100, "max. message size to send/receive to/from clients (in MB)")

	ctl.network = cmd.String("ctl-network", "unix", "control server network type: tcp, tcp4, tcp6, unix or unixpacket.")
	ctl.address = cmd.String("ctl-address", "/var/run/bblfshctl.sock", "control server address to listen.")

	log.level = cmd.String("log-level", "info", "log level: panic, fatal, error, warning, info, debug.")
	log.format = cmd.String("log-format", "text", "format of the logs: text or json.")
	log.fields = cmd.String("log-fields", "", "extra fields to add to every log line in json format.")
	cmd.Parse(os.Args[1:])

	buildLogger()
	runtime.Bootstrap()
}

func main() {
	logrus.Infof("bblfshd version: %s (build: %s)", version, build)

	r := buildRuntime()
	d := daemon.NewDaemon(version, r)
	d.Options = buildGRPCOptions()

	wg.Add(2)
	go listenUser(d)
	go listenControl(d)
	handleGracefullyShutdown(d)
	wg.Wait()
}

func listenUser(d *daemon.Daemon) {
	defer wg.Done()

	var err error
	usrListener, err = net.Listen(*network, *address)
	if err != nil {
		logrus.Errorf("error creating listener: %s", err)
		os.Exit(1)
	}

	allowAnyoneInUnixSocket(*network, *address)
	logrus.Infof("server listening in %s (%s)", *address, *network)
	if err = d.Serve(usrListener); err != nil {
		logrus.Errorf("error starting server: %s", err)
		os.Exit(1)
	}
}

func listenControl(d *daemon.Daemon) {
	defer wg.Done()

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

func buildGRPCOptions() []grpc.ServerOption {
	size := *maxMessageSize
	if size >= 2048 {
		// Setting the hard limit of message size to less than 2GB since
		// it may overflow an int value, and it should be big enough
		logrus.Errorf("max-message-size too big (limit is 2047MB): %d", size)
		os.Exit(1)
	}

	size = size * 1024 * 1024

	return []grpc.ServerOption{
		grpc.MaxRecvMsgSize(size),
		grpc.MaxSendMsgSize(size),
	}
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
