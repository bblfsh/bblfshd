package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bblfsh/bblfshd/daemon/protocol"
	"gopkg.in/bblfsh/sdk.v1/manifest/discovery"

	"github.com/briandowns/spinner"
)

var (
	// DefaultTransport is the default transport used when is missing on the
	// image reference.
	DefaultTransport = "docker://"

	SupportedTransports = map[string]bool{
		"docker":        true,
		"docker-daemon": true,
	}
)

var (
	drivers struct {
		sync.Once
		List []discovery.Driver
	}
)

func getOfficialDrivers() []discovery.Driver {
	drivers.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		list, err := discovery.OfficialDrivers(ctx, &discovery.Options{
			NoMaintainers: true,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error, %s\n", err)
		} else {
			drivers.List = list
		}
	})
	return drivers.List
}

func driverImage(id string) string {
	return fmt.Sprintf("docker://bblfsh/%s-driver:latest", id)
}

// allDrivers returns the list of all the official bblfsh drivers that are usable.
func allDrivers() map[string]string {
	list := getOfficialDrivers()
	m := make(map[string]string, len(list))
	for _, d := range list {
		if d.InDevelopment() {
			continue
		}
		m[d.Language] = driverImage(d.Language)
	}
	return m
}

// recommendedDrivers returns the list of drivers in beta state or better.
func recommendedDrivers() map[string]string {
	list := getOfficialDrivers()
	m := make(map[string]string, len(list))
	for _, d := range list {
		if !d.IsRecommended() {
			continue
		}
		m[d.Language] = driverImage(d.Language)
	}
	return m
}

const (
	DriverInstallCommandDescription = "Installs a new driver for a given language"
	DriverInstallCommandHelp        = DriverInstallCommandDescription + "\n\n" +
		"Using `--all` all the official bblfsh driver are install in the \n" +
		"daemon. Using `--recommended` will only install the recommended, \n" +
		"more developed. Using `language` and `image` positional arguments \n" +
		"one single driver can be installed or updated.\n\n" +
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
		for lang, image := range allDrivers() {
			if err := c.installDriver(lang, image); err != nil {
				return err
			}
		}
	} else if c.Recommended {
		for lang, image := range recommendedDrivers() {
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
		return fmt.Errorf("error `language` and `image` positional arguments are mandatory")
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
