package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bblfsh/bblfshd/daemon/protocol"

	"github.com/briandowns/spinner"
)

var (
	// DefaultTransport is the default transport used when is missing on the
	// image reference.
	DefaultTransport = "docker://"
	// OfficialDriver represents the list of all the official bblfsh drivers.
	OfficialDriver = map[string]string{
		"python":     "docker://bblfsh/python-driver:latest",
		"java":       "docker://bblfsh/java-driver:latest",
		"bash":       "docker://bblfsh/bash-driver:latest",
		"php":        "docker://bblfsh/php-driver:latest",
		"javascript": "docker://bblfsh/javascript-driver:latest",
		"ruby":       "docker://bblfsh/ruby-driver:latest",
		"typescript": "docker://bblfsh/typescript-driver:latest",
	}

	RecommendedDriver = map[string]string{
		"python":     "docker://bblfsh/python-driver:latest",
		"java":       "docker://bblfsh/java-driver:latest",
		"javascript": "docker://bblfsh/javascript-driver:latest",
		"ruby":       "docker://bblfsh/ruby-driver:latest",
	}

	SupportedTransports = map[string]bool{
		"docker":        true,
		"docker-daemon": true,
	}
)

const (
	DriverInstallCommandDescription = "Installs a new driver for a given language"
	DriverInstallCommandHelp        = DriverInstallCommandDescription + "\n\n" +
		"Using `--all` all the official bblfsh driver are install in the \n" +
		"daemon. Using `--recommended` will only install the recommended, \n" +
		"more developed. Using `--language` and `--image` arguments one single driver \n" +
		"can be installed or updated.\n\n" +
		"Image reference format should be `[transport]name[:tag]`.\n" +
		"Defaults are 'docker://' for transport and 'latest' for tag."
)

type DriverInstallCommand struct {
	Args struct {
		Language       string `positional-arg-name:"language" description:"language supported by the driver"`
		ImageReference string `positional-arg-name:"image" description:"driver's image reference"`
	} `positional-args:"yes"`

	Update      bool `long:"update" description:"replace the current image for the language if any"`
	All         bool `long:"all" description:"installs all the official drivers"`
	Recommended bool `long:"recommended" description:"install the recommended official drivers"`

	DriverCommand
}

func (c *DriverInstallCommand) Execute(args []string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	if c.All {
		for lang, image := range OfficialDriver {
			if err := c.installDriver(lang, image); err != nil {
				return err
			}
		}
	} else if c.Recommended {
		for lang, image := range RecommendedDriver {
			if err := c.installDriver(lang, image); err != nil {
				return err
			}
		}
	} else {
		return c.installDriver(c.Args.Language, c.Args.ImageReference)
	}

	return nil
}

func (c *DriverInstallCommand) Validate() error {
	if !c.All && !c.Recommended && (c.Args.Language == "" || c.Args.ImageReference == "") {
		return fmt.Errorf("error --language and --image are mandatory")
	}

	if c.All && c.Recommended {
		return fmt.Errorf("error --all and --recommended are exclusive")
	}

	return nil
}

func (c *DriverInstallCommand) installDriver(lang, ref string) error {
	fmt.Printf("Installing %s language driver from %q... ", lang, ref)
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
	s.Start()

	ref = c.getImageReference(ref)
	r, err := c.srv.InstallDriver(context.Background(), &protocol.InstallDriverRequest{
		Language:       lang,
		ImageReference: ref,
		Update:         c.Update,
	})

	s.Stop()
	if err == nil && len(r.Errors) == 0 {
		fmt.Println("Done")
		return nil
	}

	fmt.Println("Error")
	for _, e := range r.Errors {
		fmt.Fprintf(os.Stderr, "Error, %s\n", e)
	}

	return err
}

func (c *DriverInstallCommand) getImageReference(ref string) string {
	parts := strings.SplitN(ref, ":", 2)
	if _, ok := SupportedTransports[parts[0]]; !ok {
		return DefaultTransport + ref
	}

	return ref
}
