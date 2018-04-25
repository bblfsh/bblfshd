package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"gopkg.in/bblfsh/sdk.v1/protocol"

	"github.com/hokaccha/go-prettyjson"
)

const (
	ParseCommandDescription = "Parse a file and prints the UAST or AST"
	ParseCommandHelp        = ParseCommandDescription
)

type ParseCommand struct {
	Args struct {
		File string `positional-arg-name:"filename" description:"file to parse"`
	} `positional-args:"yes"`

	Native bool `long:"native" description:"if native, the native AST will be returned"`
	UserCommand
}

func (c *ParseCommand) Execute(args []string) error {
	if err := c.UserCommand.Execute(nil); err != nil {
		return err
	}

	if c.Args.File == "" {
		return fmt.Errorf("file argument is mandatory")
	}

	content, err := ioutil.ReadFile(c.Args.File)
	if err != nil {
		return err
	}

	if c.Native {
		return c.doNative(string(content))
	}

	return c.doUAST(string(content))
}

func (c *ParseCommand) doUAST(content string) error {
	resp, err := c.srv.Parse(context.TODO(), &protocol.ParseRequest{
		Filename: filepath.Base(c.Args.File),
		Content:  string(content),
	})

	if err != nil {
		return err
	}

	printResponse(&resp.Response)
	fmt.Println(resp.UAST)
	return nil
}

func (c *ParseCommand) doNative(content string) error {
	resp, err := c.srv.NativeParse(context.TODO(), &protocol.NativeParseRequest{
		Filename: filepath.Base(c.Args.File),
		Content:  string(content),
	})

	if err != nil {
		return err
	}

	pp, err := prettyjson.Format([]byte(resp.AST))
	if err != nil {
		return err
	}

	printResponse(&resp.Response)
	fmt.Println(string(pp))
	return nil
}

func printResponse(r *protocol.Response) {
	fmt.Printf("Status: %s\n", r.Status)
	fmt.Printf("Elapsed: %s\n", r.Elapsed)
	printErrors(r.Errors)
	fmt.Println("")
}

func printErrors(errors []string) {
	if len(errors) != 0 {
		fmt.Println("Errors:")
		for _, err := range errors {
			fmt.Printf("\t- %s\n", err)
		}
	}
}
