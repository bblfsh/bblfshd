package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bblfsh/bblfshd/v2/daemon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bblfsh/bblfshd/v2/daemon/protocol"
	"github.com/bblfsh/sdk/v3/driver/manifest/discovery"

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

func getOfficialDrivers() ([]discovery.Driver, error) {
	var err error
	drivers.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		drivers.List, err = discovery.OfficialDrivers(ctx, &discovery.Options{
			NoMaintainers: true,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error, %s\n", err)
		}
	})
	return drivers.List, err
}

func driverImage(id string) string {
	return fmt.Sprintf("docker://bblfsh/%s-driver:latest", id)
}

// allDrivers returns the list of all the official bblfsh drivers that are usable.
func allDrivers() (map[string]string, error) {
	list, err := getOfficialDrivers()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(list))
	for _, d := range list {
		if d.InDevelopment() {
			continue
		}
		m[d.Language] = driverImage(d.Language)
	}
	return m, nil
}

// recommendedDrivers returns the list of drivers in beta state or better.
func recommendedDrivers() (map[string]string, error) {
	list, err := getOfficialDrivers()
	if err != nil {
		return nil, err
	} else if len(list) == 0 {
		return nil, errors.New("official drivers list is empty; try updating bblfshd")
	}
	m := make(map[string]string, len(list))
	for _, d := range list {
		if !d.IsRecommended() {
			continue
		}
		m[d.Language] = driverImage(d.Language)
	}
	if len(m) == 0 {
		return nil, errors.New("recommended drivers list is empty; try updating bblfshd")
	}
	return m, nil
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
		Language       string `positional-arg-name:"language" description:"language supported by the driver (optional)"`
		ImageReference string `positional-arg-name:"image" description:"driver's image reference"`
	} `positional-args:"yes"`

	Update      bool `long:"update" description:"replace the current image for the language if any"`
	All         bool `long:"all" description:"installs all the official drivers"`
	Recommended bool `long:"recommended" description:"install the recommended official drivers"`
	Force       bool `short:"f" long:"force" description:"ignore already installed errors"`

	DriverCommand
}

func (c *DriverInstallCommand) Execute(args []string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	if err := c.ControlCommand.Execute(nil); err != nil {
		return err
	}

	if !c.All && !c.Recommended {
		if c.Args.ImageReference == "" {
			// TODO: go-flags does not support optional arguments in first positions
			c.Args.Language, c.Args.ImageReference = "", c.Args.Language
		}
		return c.installDrivers([]driverRef{{Lang: c.Args.Language, Ref: c.Args.ImageReference}})
	}

	var (
		m   map[string]string
		err error
	)
	if c.Recommended {
		m, err = recommendedDrivers()
	} else {
		m, err = allDrivers()
	}
	if err != nil {
		return err
	}

	list := make([]driverRef, 0, len(m))
	for lang, image := range m {
		list = append(list, driverRef{Lang: lang, Ref: image})
	}
	return c.installDrivers(list)
}

func (c *DriverInstallCommand) Validate() error {
	if !c.All && !c.Recommended && (c.Args.Language == "") {
		return fmt.Errorf("error `image` positional argument is mandatory")
	}

	if c.All && c.Recommended {
		return fmt.Errorf("error --all and --recommended are exclusive")
	}

	return nil
}

type driverRef struct {
	Lang string
	Ref  string
}

func (c *DriverInstallCommand) installDrivers(refs []driverRef) error {
	if len(refs) == 0 {
		return nil
	} else if len(refs) == 1 {
		return c.installSingleDriver(refs[0])
	}
	refNum := len(refs)
	ctx := context.Background()
	const workers = 3
	type resp struct {
		Ref string
		Err error
	}
	var (
		wg   sync.WaitGroup
		last error
		errs []error
		jobs = make(chan driverRef)
		out  = make(chan resp, len(refs))
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for ref := range jobs {
				err := c.installDriver(ctx, ref)
				out <- resp{
					Ref: ref.Ref, Err: err,
				}
			}
		}()
	}

	mref := make(map[string]driverRef, len(refs))
	for _, ref := range refs {
		mref[ref.Ref] = ref
	}
	status := make(map[string]string, len(refs))

	spin := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	clist := make([][3]string, 0, len(mref))
	todo := len(refs)
	done := 0

	accept := func(r resp) {
		todo--
		str := " + "
		if r.Err != nil {
			if daemon.ErrAlreadyInstalled.Is(r.Err) && c.Force {
				done++
			} else {
				last = r.Err
				errs = append(errs, r.Err)
				str = "ERR"
			}
		} else {
			done++
		}
		status[r.Ref] = str
	}

	draining := false
	first := true
install:
	for todo >= 0 {
		clist = clist[:0]
		for _, ref := range mref {
			clist = append(clist, [3]string{ref.Lang, status[ref.Ref], ref.Ref})
		}
		sort.Slice(clist, func(i, j int) bool {
			return clist[i][0] < clist[j][0]
		})

		if first {
			first = false
		} else {
			fmt.Print(fmt.Sprintf("\033[%dA", refNum+3)) //delete previous N lines in terminal
			fmt.Print("\033[J")
		}

		if todo == 0 {
			fmt.Printf("\nInstalled %d/%d drivers:\n", done, len(clist))
		} else {
			fmt.Printf("\nInstalling %d/%d drivers:\n", todo, len(clist))
		}
		for _, cols := range clist {
			fmt.Printf("%12s %3s %s\n", cols[0], cols[1], cols[2])
		}
		if todo == 0 {
			break
		}
		fmt.Print("Please wait  ")
		spin.Start()

		if !draining {
			if len(refs) != 0 {
				cur := refs[0]
				// send jobs and receive responses
				select {
				case jobs <- cur:
					refs = refs[1:]
					status[cur.Ref] = "..."
				case r := <-out:
					accept(r)
				}
				spin.Stop()
				fmt.Println()
				continue install
			}
			// no jobs left
			close(jobs)
			draining = true
		}

		// drain responses
		r := <-out
		spin.Stop()
		fmt.Println()
		accept(r)
	}
	fmt.Println()
	if len(errs) != 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	} else {
		fmt.Println("Done")
	}
	return last
}

func (c *DriverInstallCommand) installDriver(ctx context.Context, ref driverRef) error {
	ref.Ref = c.getImageReference(ref.Ref)
	r, err := c.srv.InstallDriver(ctx, &protocol.InstallDriverRequest{
		Language:       ref.Lang,
		ImageReference: ref.Ref,
		Update:         c.Update,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
		return daemon.ErrAlreadyInstalled.New(ref.Lang, ref.Ref)
	} else if ok && st.Code() == codes.Unauthenticated {
		return daemon.ErrUnauthorized.New(ref.Lang, ref.Ref)
	} else if err == nil && len(r.Errors) == 0 {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("%v", r.Errors)
	}
	return err
}

func (c *DriverInstallCommand) installSingleDriver(ref driverRef) error {
	ltext := ""
	if ref.Lang != "" {
		ltext = fmt.Sprintf("%s language ", ref.Lang)
	}
	fmt.Printf("Installing %sdriver from %q... ", ltext, ref.Ref)
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond) // Build our new spinner
	s.Start()

	err := c.installDriver(context.Background(), ref)

	s.Stop()
	if err == nil {
		fmt.Println("Done")
		return nil
	}
	if daemon.ErrAlreadyInstalled.Is(err) && c.Force {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
		return nil
	}
	if daemon.ErrUnauthorized.Is(err) {
		if strings.Contains(ref.Ref, "://bblfsh") {
			return fmt.Errorf("driver does not exist")
		} else {
			return fmt.Errorf("driver does not exist or is private")
		}
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
