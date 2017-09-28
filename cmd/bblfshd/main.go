package main

import (
	"flag"
	"net"
	"os"
	"strings"

	"github.com/bblfsh/server"
	"github.com/bblfsh/server/runtime"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	sdk "gopkg.in/bblfsh/sdk.v1/sdk/server"
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

	logger sdk.Logger
)

func init() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	network = cmd.String("network", "tcp", "network type: tcp, tcp4, tcp6, unix or unixpacket.")
	address = cmd.String("address", "localhost:9432", "address to listen.")
	storage = cmd.String("storage", "/var/lib/bblfshd", "path where all the runtime information is stored.")
	transport = cmd.String("transport", "docker", "default transport to fetch driver images: docker or docker-daemon)")
	maxMessageSize = cmd.Int("grpc-max-message-size", 100, "max. message size to send/receive to/from clients (in MB)")
	log.level = cmd.String("log-level", "info", "log level: panic, fatal, error, warning, info, debug.")
	log.format = cmd.String("log-format", "text", "format of the logs: text or json.")
	log.fields = cmd.String("log-fields", "", "extra fields to add to every log line in json format.")

	runtime.Bootstrap()
	logger = buildLogger()
}

func main() {
	r := buildRuntime()
	s := server.NewServer(version, r)
	s.Logger = logger
	s.Options = buildGRPCOptions()
	s.Overrides = buildOverrides()

	l, err := net.Listen(*network, *address)
	if err != nil {
		logger.Errorf("error creating listener: %s", err)
		os.Exit(1)
	}

	logger.Infof("server listening in %s (%s)", *address, *network)

	err = s.Serve(l)
	if err != nil {
		logger.Errorf("error starting server: %s", err)
		os.Exit(1)
	}
}

func buildLogger() sdk.Logger {
	logger, err := sdk.LoggerFactory{
		Level:  *log.level,
		Format: *log.format,
		Fields: *log.fields,
	}.New()

	if err != nil {
		logrus.Errorf("invalid logger configuration: %s", err)
		os.Exit(1)
	}

	return logger
}

func buildRuntime() *runtime.Runtime {
	logger.Infof("initializing runtime at %s", *storage)

	r := runtime.NewRuntime(*storage)
	if err := r.Init(); err != nil {
		logger.Errorf("error initializing runtime: %s", err)
		os.Exit(1)
	}

	return r
}

func buildGRPCOptions() []grpc.ServerOption {
	size := *maxMessageSize
	if size >= 2048 {
		// Setting the hard limit of message size to less than 2GB since
		// it may overflow an int value, and it should be big enough
		logger.Errorf("max-message-size too big (limit is 2047MB): %d", size)
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
			logger.Errorf("invalid image driver format %s", img)
			os.Exit(1)
		}

		lang := strings.TrimSpace(fields[0])
		image := strings.TrimSpace(fields[1])
		logger.Infof("Overriding image for %s: %s", lang, image)
		overrides[lang] = image
	}

	return overrides
}
