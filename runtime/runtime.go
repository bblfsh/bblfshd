package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
)

const (
	storagePath    = "images"
	containersPath = "containers"
	temporalPath   = "tmp"
)

type ConfigFactory func(containerID string) *configs.Config

type Runtime struct {
	ContainerConfigFactory ConfigFactory
	Root                   string

	s *storage
	f libcontainer.Factory
}

// NewRuntime create a new runtime using as storage the given path.
func NewRuntime(path string) *Runtime {
	return &Runtime{
		ContainerConfigFactory: ContainerConfigFactory,

		Root: path,
		s: newStorage(
			filepath.Join(path, storagePath),
			filepath.Join(path, temporalPath),
		),
	}
}

// Init initialize the runtime.
func (r *Runtime) Init() error {
	var err error
	r.f, err = libcontainer.New(
		filepath.Join(r.Root, containersPath),
		libcontainer.Cgroupfs,
	)

	return err
}

// InstallDriver installs a DriverImage extracting his content to the storage,
// only one version per image can be stored, update is required to overwrite a
// previous image if already exists otherwise, Install fails if an previous
// image already exists.
func (r *Runtime) InstallDriver(d DriverImage, update bool) (*DriverImageStatus, error) {
	return r.s.Install(d, update)
}

// RemoveDriver removes a given DriverImage from the image storage.
func (r *Runtime) RemoveDriver(d DriverImage) error {
	return r.s.Remove(d)
}

// ListDrivers lists all the driver images installed on the storage.
func (r *Runtime) ListDrivers() ([]*DriverImageStatus, error) {
	return r.s.List()
}

// Container returns a container for the given DriverImage and Process
func (r *Runtime) Container(id string, d DriverImage, p *Process, f ConfigFactory) (Container, error) {
	if f == nil {
		f = r.ContainerConfigFactory
	}

	cfg := f(id)

	var err error
	cfg.Rootfs, err = r.s.RootFS(d)
	if err != nil {
		return nil, err
	}

	imgConfig, err := ReadImageConfig(cfg.Rootfs)
	if err != nil {
		return nil, err
	}

	c, err := r.f.Create(id, cfg)
	if err != nil {
		return nil, err
	}

	return newContainer(c, p, imgConfig), nil
}

// ContainerConfigFactory is the default container config factory, is returns a
// config.Config, with the default setup.
func ContainerConfigFactory(containerID string) *configs.Config {
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

	return &configs.Config{
		Rootless: true,
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWUTS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
			{Type: configs.NEWUSER},
		}),
		UidMappings: []configs.IDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []configs.IDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
		Cgroups: &configs.Cgroup{
			Name:   containerID,
			Parent: "system",
			Resources: &configs.Resources{
				MemorySwappiness: nil,
				AllowAllDevices:  nil,
				AllowedDevices:   configs.DefaultSimpleDevices,
			},
		},
		MaskPaths: []string{
			"/proc/kcore",
			"/sys/firmware",
		},
		ReadonlyPaths: []string{
			"/proc/sys", "/proc/sysrq-trigger", "/proc/irq", "/proc/bus",
		},
		Devices:  configs.DefaultSimpleDevices,
		Hostname: containerID,
		Mounts: []*configs.Mount{
			{
				Source:      "proc",
				Destination: "/proc",
				Device:      "proc",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "tmpfs",
				Destination: "/dev",
				Device:      "tmpfs",
				Flags:       syscall.MS_NOSUID | syscall.MS_STRICTATIME,
				Data:        "mode=755",
			},
			{
				Source:      "devpts",
				Destination: "/dev/pts",
				Device:      "devpts",
				Flags:       syscall.MS_NOSUID | syscall.MS_NOEXEC,
				Data:        "newinstance,ptmxmode=0666,mode=0620",
			},
			{
				Source:      "mqueue",
				Destination: "/dev/mqueue",
				Device:      "mqueue",
				Flags:       defaultMountFlags,
			},
			//{
			//	Source:      "sysfs",
			//	Destination: "/sys",
			//	Device:      "sysfs",
			//	Flags:       defaultMountFlags | syscall.MS_RDONLY,
			//},
			{
				Source:      "/etc/localtime",
				Destination: "/etc/localtime",
				Device:      "bind",
				Flags:       syscall.MS_BIND | syscall.MS_RDONLY,
			},
		},
		Rlimits: []configs.Rlimit{
			{
				Type: syscall.RLIMIT_NOFILE,
				Hard: uint64(1025),
				Soft: uint64(1025),
			},
		},
	}
}

// Bootstrap perform the init process of a container. This function should be
// called at the init function of the application.
//
// Because containers are spawned in a two step process you will need a binary
// that will be executed as the init process for the container. In libcontainer,
// we use the current binary (/proc/self/exe) to be executed as the init
// process, and use arg "init", we call the first step process "bootstrap", so
// you always need a "init" function as the entry of "bootstrap".
//
// In addition to the go init function the early stage bootstrap is handled by
// importing nsenter.
//
// https://github.com/opencontainers/runc/blob/master/libcontainer/README.md
func Bootstrap() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			if strings.Contains(err.Error(), "permission denied") {
				panic("error bootstraping container " +
					"(hint: if SELinux is enabled, compile and load the policy module " +
					"in this repo's selinux/ directory): " + err.Error())
			} else {
				panic(err)
			}
		}
		panic("--this line should have never been executed, congratulations--")
	}
}
