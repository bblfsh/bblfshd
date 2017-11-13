package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/bblfsh/bblfshd/daemon/protocol"

	"github.com/sirupsen/logrus"
	sdk "gopkg.in/bblfsh/sdk.v1/protocol"
)

type Service struct {
	daemon *Daemon
}

func NewService(d *Daemon) *Service {
	return &Service{daemon: d}
}

func (d *Service) Parse(req *sdk.ParseRequest) *sdk.ParseResponse {
	resp := &sdk.ParseResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, req.Language, len(req.Content), resp.Elapsed)
	}()

	if req.Content == "" {
		logrus.Debugf("empty request received, returning empty UAST")
		return resp
	}

	language, dp, err := d.selectPool(req.Language, req.Content, req.Filename)
	if err != nil {
		logrus.Errorf("error selecting pool: %s", err)
		resp.Response = newResponseFromError(err)
		resp.Language = language
		return resp
	}

	req.Language = language

	err = dp.Execute(func(driver Driver) error {
		resp, err = driver.Service().Parse(context.Background(), req)
		return err
	}, req.Timeout)

	if err != nil {
		resp = &sdk.ParseResponse{}
		resp.Response = newResponseFromError(err)
	}

	resp.Language = language
	return resp
}

func (d *Service) logResponse(s sdk.Status, language string, size int, elapsed time.Duration) {
	l := logrus.WithFields(logrus.Fields{
		"language": language,
		"elapsed":  elapsed,
	})

	text := fmt.Sprintf("request processed content %d bytes, status %s", size, s)

	switch s {
	case sdk.Ok:
		l.Debug(text)
	case sdk.Error:
		l.Warning(text)
	case sdk.Fatal:
		l.Error(text)
	}
}

func (d *Service) NativeParse(req *sdk.NativeParseRequest) *sdk.NativeParseResponse {
	resp := &sdk.NativeParseResponse{}
	start := time.Now()
	defer func() {
		resp.Elapsed = time.Since(start)
		d.logResponse(resp.Status, req.Language, len(req.Content), resp.Elapsed)
	}()

	if req.Content == "" {
		logrus.Debugf("empty request received, returning empty AST")
		return resp
	}

	language, dp, err := d.selectPool(req.Language, req.Content, req.Filename)
	if err != nil {
		logrus.Errorf("error selecting pool: %s", err)
		resp.Response = newResponseFromError(err)
		return resp
	}

	req.Language = language

	err = dp.Execute(func(driver Driver) error {
		resp, err = driver.Service().NativeParse(context.Background(), req)
		return err
	}, req.Timeout)

	if err != nil {
		resp = &sdk.NativeParseResponse{}
		resp.Response = newResponseFromError(err)
	}

	resp.Language = language
	return resp
}

func (s *Service) selectPool(language, content, filename string) (string, *DriverPool, error) {
	if language == "" {
		language = GetLanguage(filename, []byte(content))
		if language == "" {
			return language, nil, ErrLanguageDetection.New()
		}
		logrus.Debugf("detected language %q, filename %q", language, filename)
	}

	dp, err := s.daemon.DriverPool(language)
	if err != nil {
		return language, nil, ErrUnexpected.Wrap(err)
	}

	return language, dp, nil
}

func (d *Service) Version(req *sdk.VersionRequest) *sdk.VersionResponse {
	return &sdk.VersionResponse{Version: d.daemon.version}
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
