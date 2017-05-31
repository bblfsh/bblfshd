package server

import (
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func TestDefaultDriverImageReference(t *testing.T) {
	require := require.New(t)

	require.Equal("docker://bblfsh/python-driver:latest", DefaultDriverImageReference("docker", "python"))
	require.Equal("docker://bblfsh/python-driver:latest", DefaultDriverImageReference("", "python"))
	require.Equal("docker-daemon:bblfsh/python-driver:latest", DefaultDriverImageReference("docker-daemon", "python"))
}
