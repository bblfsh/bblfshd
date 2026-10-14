package cmd

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

func TestUserCommandAddressResolution(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cmd := &UserCommand{}
		require.Equal(t, "tcp", cmd.targetNetwork())
		require.Equal(t, "localhost:9432", cmd.targetAddress())
	})

	t.Run("custom address", func(t *testing.T) {
		cmd := &UserCommand{Address: "192.168.1.10:9432"}
		require.Equal(t, "tcp", cmd.targetNetwork())
		require.Equal(t, "192.168.1.10:9432", cmd.targetAddress())
	})

	t.Run("custom endpoint host port", func(t *testing.T) {
		cmd := &UserCommand{Endpoint: "remote-server:9432"}
		require.Equal(t, "tcp", cmd.targetNetwork())
		require.Equal(t, "remote-server:9432", cmd.targetAddress())
	})

	t.Run("legacy endpoint network override", func(t *testing.T) {
		cmd := &UserCommand{Endpoint: "unix", Address: "/tmp/custom.sock"}
		require.Equal(t, "unix", cmd.targetNetwork())
		require.Equal(t, "/tmp/custom.sock", cmd.targetAddress())
	})

	t.Run("explicit network", func(t *testing.T) {
		cmd := &UserCommand{Network: "unix", Address: "/tmp/bblfsh.sock"}
		require.Equal(t, "unix", cmd.targetNetwork())
		require.Equal(t, "/tmp/bblfsh.sock", cmd.targetAddress())
	})
}

func TestUserCommandFlags(t *testing.T) {
	t.Run("parse address flag", func(t *testing.T) {
		cmd := &UserCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"--address", "custom-host:9432"})
		require.NoError(t, err)
		require.Equal(t, "custom-host:9432", cmd.targetAddress())
	})

	t.Run("parse endpoint flag as address", func(t *testing.T) {
		cmd := &UserCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"--endpoint", "endpoint-host:9432"})
		require.NoError(t, err)
		require.Equal(t, "endpoint-host:9432", cmd.targetAddress())
	})

	t.Run("parse network flag", func(t *testing.T) {
		cmd := &UserCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"--network", "unix"})
		require.NoError(t, err)
		require.Equal(t, "unix", cmd.targetNetwork())
	})
}
