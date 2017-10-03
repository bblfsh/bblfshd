package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bblfsh/server/daemon"
	"github.com/bblfsh/server/runtime"

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
	log            struct {
		level  *string
		format *string
		fields *string
	}
)

func init() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	network = cmd.String("network", "tcp", "network type: tcp, tcp4, tcp6, unix or unixpacket.")
	address = cmd.String("address", "0.0.0.0:9432", "address to listen.")
	storage = cmd.String("storage", "/var/lib/bblfshd", "path where all the runtime information is stored.")
	transport = cmd.String("transport", "docker", "default transport to fetch driver images: docker or docker-daemon)")
	maxMessageSize = cmd.Int("grpc-max-message-size", 100, "max. message size to send/receive to/from clients (in MB)")
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
	s := daemon.NewDaemon(version, r)
	s.Options = buildGRPCOptions()
	s.Overrides = buildOverrides()

	l, err := net.Listen(*network, *address)
	if err != nil {
		logrus.Errorf("error creating listener: %s", err)
		os.Exit(1)
	}

	logrus.Infof("server listening in %s (%s)", *address, *network)
	handleGracefullyShutdown(l, s)

	if err = s.Serve(l); err != nil {
		logrus.Errorf("error starting server: %s", err)
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

func buildOverrides() map[string]string {
	overrides := make(map[string]string)
	for _, img := range strings.Split(os.Getenv("BBLFSH_DRIVER_IMAGES"), ";") {
		if img = strings.TrimSpace(img); img == "" {
			continue
		}

		fields := strings.Split(img, "=")
		if len(fields) != 2 {
			logrus.Errorf("invalid image driver format %s", img)
			os.Exit(1)
		}

		lang := strings.TrimSpace(fields[0])
		image := strings.TrimSpace(fields[1])
		logrus.Warningf("image for %s overrided with: %q", lang, image)
		overrides[lang] = image
	}

	return overrides
}

func handleGracefullyShutdown(l net.Listener, d *daemon.Daemon) {
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	go func() {
		sig := <-gracefulStop
		logrus.Warningf("signal received %+v", sig)
		logrus.Warningf("stopping server")
		if err := d.Stop(); err != nil {
			logrus.Errorf("error stopping server: %s", err)
		}

		os.Exit(0)
	}()
}
