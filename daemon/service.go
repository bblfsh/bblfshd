// +build linux,cgo

package daemon

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
	"unicode/utf8"

	"gopkg.in/src-d/go-log.v1"

	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"github.com/bblfsh/sdk/v3/driver/manifest"
	protocol2 "github.com/bblfsh/sdk/v3/protocol"
	xcontext "golang.org/x/net/context"
	manifest1 "gopkg.in/bblfsh/sdk.v1/manifest"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
)

var (
	_ protocol2.DriverServer     = (*ServiceV2)(nil)
	_ protocol2.DriverHostServer = (*ServiceV2)(nil)

	parseKillDelay = time.Second
)

func hashSHA1(content string) string {
	h := sha1.New()
	io.WriteString(h, content)
	return hex.EncodeToString(h.Sum(nil))
}

func hashGit(content string) string {
	h := sha1.New()
	io.WriteString(h, "blob ")
	io.WriteString(h, strconv.Itoa(len(content)))
	io.WriteString(h, "\x00")
	io.WriteString(h, content)
	return hex.EncodeToString(h.Sum(nil))
}

type ServiceV2 struct {
	daemon *Daemon
}

func NewServiceV2(d *Daemon) *ServiceV2 {
	return &ServiceV2{daemon: d}
}

// Parse implements protocol2.DriverServer.
func (s *ServiceV2) Parse(rctx xcontext.Context, req *protocol2.ParseRequest) (resp *protocol2.ParseResponse, gerr error) {
	parseCallsV2.Add(1)
	defer prometheus.NewTimer(parseLatencyV2).ObserveDuration()
	parseContentSizeV2.Observe(float64(len(req.Content)))

	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.v2.Parse")
	defer sp.Finish()

	resp = &protocol2.ParseResponse{}
	start := time.Now()
	defer func() {
		s.logResponse(gerr, req.Filename, req.Language, req.Content, time.Since(start))
	}()

	if req.Content == "" {
		log.Debugf("empty request received, returning empty UAST")
		return resp, nil
	}

	if !utf8.ValidString(req.Content) {
		parseErrorsV2.Add(1)
		err := ErrUnknownEncoding.New()
		log.Debugf("parse v2 (%s): %s", req.Filename, err)
		return nil, err
	}

	language, dp, err := s.selectPool(ctx, req.Language, req.Content, req.Filename)
	if err != nil {
		parseErrorsV2.Add(1)
		log.Errorf(err, "error selecting pool")
		return nil, err
	}

	req.Language = language

	err = dp.ExecuteCtx(ctx, func(ctx context.Context, driver Driver) error {
		resp, err = parseV2(ctx, dp, driver, req)
		return err
	})
	if err != nil {
		parseErrorsV2.Add(1)
	}
	if resp != nil {
		resp.Language = language
	}
	return resp, err
}

func parseV2(ctx context.Context, pool *DriverPool, drv Driver, req *protocol2.ParseRequest) (*protocol2.ParseResponse, error) {
	var (
		resp *protocol2.ParseResponse
		err  error
	)
	done := make(chan struct{})
	go func() {
		resp, err = drv.ServiceV2().Parse(ctx, req)
		close(done)
	}()

	var (
		ctxKill context.Context
		cancel  context.CancelFunc
	)
	if deadline, ok := ctx.Deadline(); ok {
		ctxKill, cancel = context.WithDeadline(context.Background(), deadline.Add(parseKillDelay))
		defer cancel()
	} else {
		ctxKill = ctx
	}

	select {
	case <-done:
		return resp, err

	case <-ctxKill.Done():
		pool.killDriver(drv, "parseV2", ctxKill.Err())
		return nil, ctxKill.Err()
	}
}

// ServerVersion implements protocol2.DriverHostServer.
func (s *ServiceV2) ServerVersion(rctx xcontext.Context, _ *protocol2.VersionRequest) (*protocol2.VersionResponse, error) {
	versionCallsV2.Add(1)

	sp, _ := opentracing.StartSpanFromContext(rctx, "bblfshd.v2.ServerVersion")
	defer sp.Finish()

	resp := &protocol2.Version{Version: s.daemon.version}
	if !s.daemon.build.IsZero() {
		resp.Build = s.daemon.build
	}
	return &protocol2.VersionResponse{Version: resp}, nil
}

// SupportedLanguages implements protocol2.DriverHostServer.
func (s *ServiceV2) SupportedLanguages(rctx xcontext.Context, _ *protocol2.SupportedLanguagesRequest) (_ *protocol2.SupportedLanguagesResponse, gerr error) {
	languagesCallsV2.Add(1)

	sp, _ := opentracing.StartSpanFromContext(rctx, "bblfshd.v2.SupportedLanguages")
	defer sp.Finish()

	start := time.Now()
	defer func() {
		s.logResponse(gerr, "", "", "", time.Since(start))
	}()

	drivers, err := s.daemon.runtime.ListDrivers()
	if err != nil {
		return nil, err
	}

	out := make([]*protocol2.Manifest, 0, len(drivers))
	for _, d := range drivers {
		if d.Manifest == nil {
			return nil, errors.New("empty manifest returned by driver")
		}
		out = append(out, protocol2.NewManifest(d.Manifest))
	}
	return &protocol2.SupportedLanguagesResponse{Languages: out}, nil
}

func (s *ServiceV2) logResponse(err error, filename, language, content string, elapsed time.Duration) {
	fields := log.Fields{"elapsed": elapsed}
	if filename != "" {
		fields["filename"] = filename
	}

	if language != "" {
		fields["language"] = language
	}

	if content != "" {
		fields["sha1"] = hashSHA1(content)
		fields["githash"] = hashGit(content)
	}

	l := log.With(fields)
	text := fmt.Sprintf("request processed content %d bytes", len(content))

	if err != nil {
		l.Errorf(err, "%s", text)
	} else {
		l.Infof("%s", text)
	}
}

func (s *ServiceV2) detectLanguage(rctx context.Context, content, filename string) (string, error) {
	sp, _ := opentracing.StartSpanFromContext(rctx, "bblfshd.detectLanguage")
	defer sp.Finish()

	language := GetLanguage(filename, []byte(content))
	if language == "" {
		return "", ErrLanguageDetection.New()
	}
	log.Debugf("detected language %q, filename %q", language, filename)
	return language, nil
}

func (s *ServiceV2) selectPool(rctx context.Context, language, content, filename string) (string, *DriverPool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.pool.select")
	defer sp.Finish()

	if language == "" {
		lang, err := s.detectLanguage(ctx, content, filename)
		if err != nil {
			return "", nil, err
		}
		language = lang
	} else { // always re-map enry->bblfsh language names
		language = normalize(language)
	}

	dp, err := s.daemon.DriverPool(ctx, language)
	if err != nil {
		return language, nil, ErrUnexpected.Wrap(err)
	}

	return language, dp, nil
}

var _ protocol1.Service = (*Service)(nil)

type Service struct {
	daemon *Daemon
}

func NewService(d *Daemon) *Service {
	return &Service{daemon: d}
}

// Parse implements protocol1.Service.
func (d *Service) Parse(req *protocol1.ParseRequest) *protocol1.ParseResponse {
	parseCallsV1.Add(1)
	parseContentSizeV1.Observe(float64(len(req.Content)))

	resp := &protocol1.ParseResponse{}
	start := time.Now()
	defer func() {
		if resp == nil || resp.Status != protocol1.Ok || len(resp.Errors) != 0 {
			parseErrorsV1.Add(1)
		}
		dt := time.Since(start)
		parseLatencyV1.Observe(dt.Seconds())
		resp.Elapsed = dt
		d.logResponse(resp.Status, req.Filename, req.Language, req.Content, resp.Elapsed)
	}()

	if req.Content == "" {
		log.Debugf("empty request received, returning empty UAST")
		return resp
	}
	if !utf8.ValidString(req.Content) {
		err := ErrUnknownEncoding.New()
		log.Debugf("parse v1 (%s): %s", req.Filename, err)
		resp.Response = newResponseFromError(err)
		return resp
	}

	language, dp, err := d.selectPool(context.TODO(), req.Language, req.Content, req.Filename)
	if err != nil {
		log.Errorf(err, "error selecting pool")
		resp.Response = newResponseFromError(err)
		resp.Language = language
		return resp
	}

	req.Language = language

	err = dp.Execute(func(ctx context.Context, driver Driver) error {
		resp, err = parseV1(ctx, dp, driver, req)
		return err
	}, req.Timeout)

	if err != nil {
		resp = &protocol1.ParseResponse{}
		resp.Response = newResponseFromError(err)
	}

	resp.Language = language
	return resp
}

func parseV1(ctx context.Context, pool *DriverPool, drv Driver, req *protocol1.ParseRequest) (*protocol1.ParseResponse, error) {
	var (
		resp *protocol1.ParseResponse
		err  error
	)
	done := make(chan struct{})
	go func() {
		resp, err = drv.Service().Parse(ctx, req)
		close(done)
	}()

	var (
		ctxKill context.Context
		cancel  context.CancelFunc
	)
	if deadline, ok := ctx.Deadline(); ok {
		ctxKill, cancel = context.WithDeadline(context.Background(), deadline.Add(parseKillDelay))
		defer cancel()
	} else {
		ctxKill = ctx
	}

	select {
	case <-done:
		return resp, err

	case <-ctxKill.Done():
		pool.killDriver(drv, "parseV1", ctxKill.Err())
		return nil, ctxKill.Err()
	}
}

func (d *Service) logResponse(s protocol1.Status, filename, language, content string, elapsed time.Duration) {
	fields := log.Fields{"elapsed": elapsed}
	if filename != "" {
		fields["filename"] = filename
	}

	if language != "" {
		fields["language"] = language
	}

	if content != "" {
		fields["sha1"] = hashSHA1(content)
		fields["githash"] = hashGit(content)
	}

	l := log.With(fields)
	text := fmt.Sprintf("request processed content %d bytes, status %s", len(content), s)

	switch s {
	case protocol1.Ok:
		l.Infof("%s", text)
	case protocol1.Error:
		l.Warningf("%s", text)
	case protocol1.Fatal:
		l.Errorf(errors.New("protocol1 fatal error"), "%s", text)
	}
}

// NativeParse implements protocol1.Service.
func (d *Service) NativeParse(req *protocol1.NativeParseRequest) *protocol1.NativeParseResponse {
	parseCallsV1.Add(1)
	parseContentSizeV1.Observe(float64(len(req.Content)))

	resp := &protocol1.NativeParseResponse{}
	start := time.Now()
	defer func() {
		if resp == nil || resp.Status != protocol1.Ok || len(resp.Errors) != 0 {
			parseErrorsV1.Add(1)
		}
		dt := time.Since(start)
		parseLatencyV1.Observe(dt.Seconds())
		resp.Elapsed = dt
		d.logResponse(resp.Status, req.Language, req.Language, req.Content, resp.Elapsed)
	}()

	if req.Content == "" {
		log.Debugf("empty request received, returning empty AST")
		return resp
	}

	if !utf8.ValidString(req.Content) {
		err := ErrUnknownEncoding.New()
		log.Debugf("native parse v1 (%s): %s", req.Filename, err)
		resp.Response = newResponseFromError(err)
		return resp
	}

	language, dp, err := d.selectPool(context.TODO(), req.Language, req.Content, req.Filename)
	if err != nil {
		log.Errorf(err, "error selecting pool")
		resp.Response = newResponseFromError(err)
		return resp
	}

	req.Language = language

	err = dp.Execute(func(ctx context.Context, driver Driver) error {
		resp, err = driver.Service().NativeParse(ctx, req)
		return err
	}, req.Timeout)

	if err != nil {
		resp = &protocol1.NativeParseResponse{}
		resp.Response = newResponseFromError(err)
	}

	resp.Language = language
	return resp
}

func (s *Service) selectPool(ctx context.Context, language, content, filename string) (string, *DriverPool, error) {
	if language == "" {
		language = GetLanguage(filename, []byte(content))
		if language == "" {
			return language, nil, ErrLanguageDetection.New()
		}
		log.Debugf("detected language %q, filename %q", language, filename)
	} else { // always re-map enry->bblfsh language names
		language = normalize(language)
	}

	dp, err := s.daemon.DriverPool(ctx, language)
	if err != nil {
		return language, nil, ErrUnexpected.Wrap(err)
	}

	return language, dp, nil
}

// Version implements protocol1.Service.
func (d *Service) Version(req *protocol1.VersionRequest) *protocol1.VersionResponse {
	versionCallsV1.Add(1)

	resp := &protocol1.VersionResponse{Version: d.daemon.version, Build: d.daemon.build}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, "", "", "", resp.Elapsed)
	}()
	return resp
}

// manifestV2toV1 converts driver manifest from v2 API format to v1.
func manifestV2toV1(m *manifest.Manifest) *manifest1.Manifest {
	dm := &manifest1.Manifest{
		Name:     m.Name,
		Language: m.Language,
		Version:  m.Version,
		Status:   manifest1.DevelopmentStatus(m.Status),
		Features: make([]manifest1.Feature, 0, len(m.Features)),
	}
	dm.Runtime.GoVersion = m.Runtime.GoVersion
	dm.Runtime.NativeVersion = []string{m.Runtime.NativeVersion}
	if m.Documentation != nil {
		dm.Documentation.Description = m.Documentation.Description
		dm.Documentation.Caveats = m.Documentation.Caveats
	}
	if !m.Build.IsZero() {
		dm.Build = &m.Build
	}
	for _, f := range m.Features {
		dm.Features = append(dm.Features, manifest1.Feature(f))
	}
	return dm
}

// SupportedLanguages implements protocol1.Service.
func (d *Service) SupportedLanguages(req *protocol1.SupportedLanguagesRequest) *protocol1.SupportedLanguagesResponse {
	languagesCallsV1.Add(1)

	resp := &protocol1.SupportedLanguagesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, "", "", "", resp.Elapsed)
	}()

	drivers, err := d.daemon.runtime.ListDrivers()
	if err != nil {
		resp.Response = newResponseFromError(err)
		return resp
	}

	driverRes := make([]protocol1.DriverManifest, len(drivers))
	for i, driver := range drivers {
		m := manifestV2toV1(driver.Manifest)
		driverRes[i] = protocol1.NewDriverManifest(m)
	}

	resp.Languages = driverRes
	return resp
}

type ControlService struct {
	*Daemon
}

func NewControlService(d *Daemon) *ControlService {
	return &ControlService{Daemon: d}
}

func (s *ControlService) DriverPoolStates() map[string]*protocol.DriverPoolState {
	out := make(map[string]*protocol.DriverPoolState, 0)
	for language, pool := range s.Daemon.Current() {
		out[language] = pool.State()
	}

	return out
}

func (s *ControlService) DriverInstanceStates() ([]*protocol.DriverInstanceState, error) {
	var out []*protocol.DriverInstanceState
	for _, pool := range s.Daemon.Current() {
		for _, driver := range pool.Current() {
			status, err := driver.State()
			if err != nil {
				return nil, err
			}

			out = append(out, status)
		}
	}

	return out, nil
}

func (s *ControlService) DriverStates() ([]*protocol.DriverImageState, error) {
	list, err := s.Daemon.runtime.ListDrivers()
	if err != nil {
		return nil, err
	}

	var out []*protocol.DriverImageState
	for _, d := range list {
		out = append(out, &protocol.DriverImageState{
			Reference:     d.Reference,
			Language:      d.Manifest.Language,
			Version:       d.Manifest.Version,
			Build:         d.Manifest.Build,
			Status:        string(d.Manifest.Status),
			GoVersion:     string(d.Manifest.Runtime.GoVersion),
			NativeVersion: []string{d.Manifest.Runtime.NativeVersion},
		})
	}

	return out, nil
}
