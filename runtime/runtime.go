//go:build linux && cgo
// +build linux,cgo

package runtime

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/cgroups/devices/config"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/opencontainers/runc/libcontainer/specconv"
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
	return nil
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

	c, err := libcontainer.Create(
		filepath.Join(r.Root, containersPath),
		// TODO:  libcontainer.RootlessCgroupfs,
		id, cfg,
	)
	if err != nil {
		return nil, err
	}

	return newContainer(c, p, imgConfig), nil
}

// ContainerConfigFactory is the default container config factory, is returns a
// config.Config, with the default setup.
func ContainerConfigFactory(containerID string) *configs.Config {
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV
	var devices []*config.Rule
	for _, device := range specconv.AllowedDevices {
		devices = append(devices, &device.Rule)
	}
	return &configs.Config{
		RootlessEUID:    true,
		RootlessCgroups: true,
		Namespaces: configs.Namespaces{
			{Type: configs.NEWNS},
			{Type: configs.NEWUTS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
			{Type: configs.NEWUSER},
		},
		UIDMappings: []configs.IDMap{
			{ContainerID: 0, HostID: int64(os.Getuid()), Size: 1},
		},
		GIDMappings: []configs.IDMap{
			{ContainerID: 0, HostID: int64(os.Getgid()), Size: 1},
		},
		Cgroups: &cgroups.Cgroup{
			Name:   containerID,
			Parent: "system",
			Resources: &cgroups.Resources{
				Devices: devices,
			},
		},
		MaskPaths: []string{
			"/proc/kcore",
			"/sys/firmware",
		},
		ReadonlyPaths: []string{
			"/proc/sys", "/proc/sysrq-trigger", "/proc/irq", "/proc/bus",
		},
		Devices:  specconv.AllowedDevices,
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
		libcontainer.Init()
		panic("--this line should have never been executed, congratulations--")
	}
}
