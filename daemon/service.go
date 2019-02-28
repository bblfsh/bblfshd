// +build linux,cgo

package daemon

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	xcontext "golang.org/x/net/context"
	protocol1 "gopkg.in/bblfsh/sdk.v1/protocol"
	protocol2 "gopkg.in/bblfsh/sdk.v2/protocol"
)

var _ protocol2.DriverServer = (*ServiceV2)(nil)

type ServiceV2 struct {
	daemon *Daemon
}

func NewServiceV2(d *Daemon) *ServiceV2 {
	return &ServiceV2{daemon: d}
}

func (s *ServiceV2) Parse(rctx xcontext.Context, req *protocol2.ParseRequest) (resp *protocol2.ParseResponse, gerr error) {
	sp, ctx := opentracing.StartSpanFromContext(rctx, "bblfshd.v2.Parse")
	defer sp.Finish()

	resp = &protocol2.ParseResponse{}
	start := time.Now()
	defer func() {
		s.logResponse(gerr, req.Filename, req.Language, len(req.Content), time.Since(start))
	}()

	if req.Content == "" {
		logrus.Debugf("empty request received, returning empty UAST")
		return resp, nil
	}

	if !utf8.ValidString(req.Content) {
		err := ErrUnknownEncoding.New()
		logrus.Debugf("parse v2 (%s): %s", req.Filename, err)
		return nil, err
	}

	language, dp, err := s.selectPool(ctx, req.Language, req.Content, req.Filename)
	if err != nil {
		logrus.Errorf("error selecting pool: %s", err)
		return nil, err
	}

	req.Language = language

	err = dp.ExecuteCtx(ctx, func(ctx context.Context, driver Driver) error {
		resp, err = driver.ServiceV2().Parse(ctx, req)
		return err
	})
	if resp != nil {
		resp.Language = language
	}
	return resp, err
}

func (s *ServiceV2) logResponse(err error, filename string, language string, size int, elapsed time.Duration) {
	fields := logrus.Fields{"elapsed": elapsed}
	if filename != "" {
		fields["filename"] = filename
	}

	if language != "" {
		fields["language"] = language
	}

	l := logrus.WithFields(fields)
	text := fmt.Sprintf("request processed content %d bytes", size)

	if err != nil {
		text += " error: " + err.Error()
		l.Error(text)
	} else {
		l.Debug(text)
	}
}

func (s *ServiceV2) detectLanguage(rctx context.Context, content, filename string) (string, error) {
	sp, _ := opentracing.StartSpanFromContext(rctx, "bblfshd.detectLanguage")
	defer sp.Finish()

	language := GetLanguage(filename, []byte(content))
	if language == "" {
		return "", ErrLanguageDetection.New()
	}
	logrus.Debugf("detected language %q, filename %q", language, filename)
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

func (d *Service) Parse(req *protocol1.ParseRequest) *protocol1.ParseResponse {
	resp := &protocol1.ParseResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, req.Filename, req.Language, len(req.Content), resp.Elapsed)
	}()

	if req.Content == "" {
		logrus.Debugf("empty request received, returning empty UAST")
		return resp
	}
	if !utf8.ValidString(req.Content) {
		err := ErrUnknownEncoding.New()
		logrus.Debugf("parse v1 (%s): %s", req.Filename, err)
		resp.Response = newResponseFromError(err)
		return resp
	}

	language, dp, err := d.selectPool(context.TODO(), req.Language, req.Content, req.Filename)
	if err != nil {
		logrus.Errorf("error selecting pool: %s", err)
		resp.Response = newResponseFromError(err)
		resp.Language = language
		return resp
	}

	req.Language = language

	err = dp.Execute(func(ctx context.Context, driver Driver) error {
		resp, err = driver.Service().Parse(ctx, req)
		return err
	}, req.Timeout)

	if err != nil {
		resp = &protocol1.ParseResponse{}
		resp.Response = newResponseFromError(err)
	}

	resp.Language = language
	return resp
}

func (d *Service) logResponse(s protocol1.Status, filename string, language string, size int, elapsed time.Duration) {
	fields := logrus.Fields{"elapsed": elapsed}
	if filename != "" {
		fields["filename"] = filename
	}

	if language != "" {
		fields["language"] = language
	}

	l := logrus.WithFields(fields)
	text := fmt.Sprintf("request processed content %d bytes, status %s", size, s)

	switch s {
	case protocol1.Ok:
		l.Debug(text)
	case protocol1.Error:
		l.Warning(text)
	case protocol1.Fatal:
		l.Error(text)
	}
}

func (d *Service) NativeParse(req *protocol1.NativeParseRequest) *protocol1.NativeParseResponse {
	resp := &protocol1.NativeParseResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, req.Language, req.Language, len(req.Content), resp.Elapsed)
	}()

	if req.Content == "" {
		logrus.Debugf("empty request received, returning empty AST")
		return resp
	}

	if !utf8.ValidString(req.Content) {
		err := ErrUnknownEncoding.New()
		logrus.Debugf("native parse v1 (%s): %s", req.Filename, err)
		resp.Response = newResponseFromError(err)
		return resp
	}

	language, dp, err := d.selectPool(context.TODO(), req.Language, req.Content, req.Filename)
	if err != nil {
		logrus.Errorf("error selecting pool: %s", err)
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
		logrus.Debugf("detected language %q, filename %q", language, filename)
	} else { // always re-map enry->bblfsh language names
		language = normalize(language)
	}

	dp, err := s.daemon.DriverPool(ctx, language)
	if err != nil {
		return language, nil, ErrUnexpected.Wrap(err)
	}

	return language, dp, nil
}

func (d *Service) Version(req *protocol1.VersionRequest) *protocol1.VersionResponse {
	resp := &protocol1.VersionResponse{Version: d.daemon.version, Build: d.daemon.build}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, "", "", 0, resp.Elapsed)
	}()
	return resp
}

func (d *Service) SupportedLanguages(req *protocol1.SupportedLanguagesRequest) *protocol1.SupportedLanguagesResponse {
	resp := &protocol1.SupportedLanguagesResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, "", "", 0, resp.Elapsed)
	}()

	drivers, err := d.daemon.runtime.ListDrivers()
	if err != nil {
		resp.Response = newResponseFromError(err)
		return resp
	}

	driverRes := make([]protocol1.DriverManifest, len(drivers))
	for i, driver := range drivers {
		driverRes[i] = protocol1.NewDriverManifest(driver.Manifest)
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
		build := d.Manifest.Build
		if build == nil {
			build = &time.Time{}
		}

		out = append(out, &protocol.DriverImageState{
			Reference:     d.Reference,
			Language:      d.Manifest.Language,
			Version:       d.Manifest.Version,
			Build:         *build,
			Status:        string(d.Manifest.Status),
			OS:            string(d.Manifest.Runtime.OS),
			GoVersion:     string(d.Manifest.Runtime.GoVersion),
			NativeVersion: []string(d.Manifest.Runtime.NativeVersion),
		})
	}

	return out, nil
}
