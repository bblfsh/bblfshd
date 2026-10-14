package cmd

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

func TestParseCommandFilePath(t *testing.T) {
	// Positional argument only
	c1 := &ParseCommand{}
	c1.Args.File = "test.py"
	require.Equal(t, "test.py", c1.filePath())

	// -f / --file flag only
	c2 := &ParseCommand{File: "flag.py"}
	require.Equal(t, "flag.py", c2.filePath())

	// Flag takes precedence over positional arg
	c3 := &ParseCommand{File: "flag.py"}
	c3.Args.File = "arg.py"
	require.Equal(t, "flag.py", c3.filePath())

	// Neither is set
	c4 := &ParseCommand{}
	require.Equal(t, "", c4.filePath())

	err := c4.Execute(nil)
	require.Error(t, err)
	require.Equal(t, "file argument is mandatory", err.Error())
}

func TestParseCommandFlags(t *testing.T) {
	t.Run("positional argument", func(t *testing.T) {
		cmd := &ParseCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"sample.py"})
		require.NoError(t, err)
		require.Equal(t, "sample.py", cmd.Args.File)
		require.Equal(t, "sample.py", cmd.filePath())
		require.False(t, cmd.Native)
	})

	t.Run("short flag -f", func(t *testing.T) {
		cmd := &ParseCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"-f", "sample.py"})
		require.NoError(t, err)
		require.Equal(t, "sample.py", cmd.File)
		require.Equal(t, "sample.py", cmd.filePath())
	})

	t.Run("long flag --file", func(t *testing.T) {
		cmd := &ParseCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"--file", "sample.py"})
		require.NoError(t, err)
		require.Equal(t, "sample.py", cmd.File)
		require.Equal(t, "sample.py", cmd.filePath())
	})

	t.Run("flag with native option", func(t *testing.T) {
		cmd := &ParseCommand{}
		parser := flags.NewParser(cmd, flags.Default)
		_, err := parser.ParseArgs([]string{"--native", "-f", "sample.py"})
		require.NoError(t, err)
		require.True(t, cmd.Native)
		require.Equal(t, "sample.py", cmd.filePath())
	})
}
