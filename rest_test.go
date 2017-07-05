package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bblfsh/sdk/protocol"
	"github.com/bblfsh/server/runtime"
	"github.com/stretchr/testify/require"
)

func TestRestServerParse(t *testing.T) {
	require := require.New(t)

	tmpDir, err := ioutil.TempDir(os.TempDir(), "bblfsh-runtime")
	require.NoError(err)

	defer func() { require.NoError(os.RemoveAll(tmpDir)) }()

	r := runtime.NewRuntime(tmpDir)
	err = r.Init()
	require.NoError(err)

	s := NewServer(r, make(map[string]string))
	dp, err := StartDriverPool(DefaultScalingPolicy(), DefaultPoolTimeout, func() (Driver, error) {
		return &echoDriver{}, nil
	})
	require.NoError(err)
	require.NotNil(dp)

	s.drivers["python"] = dp

	srv := &RESTServer{s}
	addr := "0.0.0.0:9999"
	go srv.Serve(addr)

	<-time.After(100 * time.Millisecond)

	data, err := json.Marshal(protocol.ParseUASTRequest{
		Filename: "foo.py",
		Content:  "foo = 1",
	})
	require.NoError(err)

	url := fmt.Sprintf("http://%s/parse", addr)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	require.NoError(err)

	var result protocol.ParseUASTResponse
	require.NoError(json.NewDecoder(resp.Body).Decode(&result))

	require.Equal(protocol.Ok, result.Status)
	require.Len(result.Errors, 0)
	require.Equal("foo = 1", result.UAST.Token)
}
