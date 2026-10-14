package cmd

import (
	"errors"
	"fmt"

	bblfsh "github.com/bblfsh/go-client/v4"
	"github.com/bblfsh/sdk/v3/uast/uastyaml"
)

const (
	ParseCommandDescription = "Parse a file and prints the UAST or AST"
	ParseCommandHelp        = ParseCommandDescription
)

type ParseCommand struct {
	Args struct {
		File string `positional-arg-name:"filename" description:"file to parse"`
	} `positional-args:"yes"`

	File string `short:"f" long:"file" description:"file to parse"`

	Native bool `long:"native" description:"if native, the native AST will be returned"`
	UserCommand
}

func (c *ParseCommand) filePath() string {
	if c.File != "" {
		return c.File
	}
	return c.Args.File
}

func (c *ParseCommand) Execute(args []string) error {
	file := c.filePath()
	if file == "" {
		return errors.New("file argument is mandatory")
	}
	if err := c.UserCommand.Execute(nil); err != nil {
		return err
	}

	cli, err := bblfsh.NewClientWithConnection(c.conn)
	if err != nil {
		return err
	}
	defer cli.Close()

	req := cli.NewParseRequest().ReadFile(file)
	if c.Native {
		req = req.Mode(bblfsh.Native)
	}

	ast, _, err := req.UAST()
	if err != nil {
		return err
	}
	data, err := uastyaml.Marshal(ast)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
