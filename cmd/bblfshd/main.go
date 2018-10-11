package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bblfsh/bblfshd/daemon"
	"github.com/bblfsh/bblfshd/runtime"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	"gopkg.in/bblfsh/sdk.v1/sdk/server"
	"gopkg.in/bblfsh/sdk.v2/driver"
	"gopkg.in/bblfsh/sdk.v2/driver/manifest/discovery"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes/nodesproto"
	"gopkg.in/bblfsh/sdk.v2/uast/query"
	"gopkg.in/bblfsh/sdk.v2/uast/query/xpath"
	"gopkg.in/bblfsh/sdk.v2/uast/yaml"
)

var (
	version = "undefined"
	build   = "undefined"

	network        *string
	address        *string
	addressHTTP    *string
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

	usrListener  net.Listener
	ctlListener  net.Listener
	httpListener net.Listener

	srv1 *daemon.Service
	srv2 *daemon.ServiceV2
)

func init() {
	cmd = flag.NewFlagSet("bblfshd", flag.ExitOnError)
	network = cmd.String("network", "tcp", "network type: tcp, tcp4, tcp6, unix or unixpacket.")
	address = cmd.String("address", "0.0.0.0:9432", "address to listen.")
	addressHTTP = cmd.String("http", "0.0.0.0:9480", "address to listen (http)")
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

	r := buildRuntime()
	d := daemon.NewDaemon(version, r, buildGRPCOptions()...)
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
	if *addressHTTP != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listenHTTP(d)
		}()
	}
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

func listenHTTP(d *daemon.Daemon) {
	var err error
	httpListener, err = net.Listen("tcp", *addressHTTP)
	if err != nil {
		logrus.Errorf("error creating http listener: %s", err)
		os.Exit(1)
	}
	srv1 = daemon.NewService(d)
	srv2 = daemon.NewServiceV2(d)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", handleVersion)
	mux.HandleFunc("/api/v1/languages", handleLanguages)
	mux.HandleFunc("/api/v1/parse", handleParse)

	hs := &http.Server{
		Handler: mux,
	}

	logrus.Infof("http server listening on http://%s", *addressHTTP)
	if err = hs.Serve(httpListener); err != nil {
		logrus.Errorf("error starting http server: %s", err)
		os.Exit(1)
	}
}

func jsonError(w http.ResponseWriter, code int, err error) {
	if code == 0 {
		code = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": err.Error(),
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp := srv1.Version(&protocol1.VersionRequest{})
	if len(resp.Errors) != 0 || resp.Status != protocol1.Ok {
		var errs []string
		for _, e := range resp.Errors {
			errs = append(errs, e)
		}
		jsonError(w, 0, errors.New(strings.Join(errs, "\n")))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	build := ""
	if !resp.Build.IsZero() {
		build = resp.Build.Format(time.RFC3339)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version": resp.Version,
		"build":   build,
	})
}

func handleLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp := srv1.SupportedLanguages(&protocol1.SupportedLanguagesRequest{})
	if len(resp.Errors) != 0 || resp.Status != protocol1.Ok {
		var errs []string
		for _, e := range resp.Errors {
			errs = append(errs, e)
		}
		jsonError(w, 0, errors.New(strings.Join(errs, "\n")))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp.Languages)
}

var httpTypes = map[string]string{
	"application/json":   "json",
	"text/yaml":          "yaml",
	"text/x-yaml":        "yaml",
	"application/x-yaml": "yaml",
	"text/vnd.yaml":      "yaml",
}

func handleParse(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()

	lang := q.Get("lang")
	file := q.Get("file")

	var mode = driver.ModeDefault
	if smode := q.Get("mode"); smode != "" {
		var err error
		mode, err = driver.ParseMode(smode)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
	}

	var qu query.Query
	if query := q.Get("query"); query != "" {
		var err error
		qu, err = xpath.New().Prepare(query)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
	}

	maxSize := *maxMessageSize * 1024 * 1024
	data, err := ioutil.ReadAll(io.LimitReader(r.Body, int64(maxSize+1)))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	} else if len(data) > maxSize {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("content is too large (> %d MB)", *maxMessageSize))
		return
	}

	resp, err := srv2.Parse(r.Context(), &protocol2.ParseRequest{
		Content:  string(data),
		Language: lang,
		Filename: file,
		Mode:     protocol2.Mode(mode),
	})
	if err != nil {
		jsonError(w, 0, err)
		return
	} else if len(resp.Errors) != 0 {
		var errs []string
		for _, e := range resp.Errors {
			errs = append(errs, e.Text)
		}
		jsonError(w, 0, errors.New(strings.Join(errs, "\n")))
		return
	}
	typ := httpTypes[r.Header.Get("Accept")]
	if typ == "" && qu == nil {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(resp.Uast)
		return
	}
	uast, err := resp.Nodes()
	if err != nil {
		jsonError(w, 0, err)
		return
	}
	if qu != nil {
		it, err := qu.Execute(uast)
		if err != nil {
			jsonError(w, 0, err)
			return
		}
		var arr nodes.Array
		for it.Next() {
			n, ok := it.Node().(nodes.Node)
			if !ok {
				jsonError(w, 0, fmt.Errorf("ensupported node type: %T", it.Node()))
				return
			}
			arr = append(arr, n)
		}
		uast = arr
	}

	switch typ {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(uast)
	case "yaml":
		w.Header().Set("Content-Type", r.Header.Get("Accept"))
		uastyml.NewEncoder(w).Encode(uast)
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
		nodesproto.WriteTo(w, uast)
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

	for _, l := range []net.Listener{ctlListener, usrListener, httpListener} {
		if l == nil {
			continue
		}
		if err := l.Close(); err != nil {
			logrus.Errorf("error closing listener: %s", err)
		}
	}

	os.Exit(0)
}
