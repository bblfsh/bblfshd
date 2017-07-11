package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/bblfsh/sdk/protocol"
	"github.com/bblfsh/server/runtime"
	"github.com/gin-gonic/gin"
	"github.com/rs/cors"
)

type RESTServer struct {
	*Server
}

func NewRESTServer(r *runtime.Runtime, overrides map[string]string, transport string) *RESTServer {
	server := NewServer(r, overrides)
	server.Transport = transport
	return &RESTServer{server}
}

func (s *RESTServer) Serve(addr string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	protocol.DefaultParser = s.Server
	r.POST("/parse", s.handleParse)
	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	})

	logrus.Info("starting REST server")
	server := &http.Server{
		Addr:         addr,
		Handler:      cors.Handler(r),
		ReadTimeout:  1 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}
	return server.ListenAndServe()
}

func (s *RESTServer) handleParse(ctx *gin.Context) {
	var req protocol.ParseRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, jsonError("unable to read request: %s", err))
		return
	}

	resp := s.Parse(&req)
	ctx.JSON(toHTTPStatus(resp.Status), resp)
}

func toHTTPStatus(status protocol.Status) int {
	switch status {
	case protocol.Ok:
		return http.StatusOK
	case protocol.Error:
		return http.StatusBadRequest
	}

	return http.StatusInternalServerError
}

func jsonError(msg string, args ...interface{}) gin.H {
	return gin.H{
		"status": protocol.Fatal,
		"errors": []gin.H{
			gin.H{
				"message": fmt.Sprintf(msg, args...),
			},
		},
	}
}
