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

	no_overrides := make(map[string]string)
	python_override := make(map[string]string)
	python_override["python"] = "overriden"

	require.Equal("docker://bblfsh/python-driver:latest", DefaultDriverImageReference(no_overrides, "docker", "python"))
	require.Equal("docker://bblfsh/python-driver:latest", DefaultDriverImageReference(no_overrides, "", "python"))
	require.Equal("docker-daemon:bblfsh/python-driver:latest", DefaultDriverImageReference(no_overrides, "docker-daemon", "python"))
	require.Equal("overriden", DefaultDriverImageReference(python_override, "docker-daemon", "python"))
}
