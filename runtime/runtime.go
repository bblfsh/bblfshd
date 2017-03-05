package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/imdario/go-ulid"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
)

const (
	storagePath    = "images"
	containersPath = "containers"
)

type Runtime struct {
	ContainerConfigFactory func() *configs.Config

	root string

	s *storage
	f libcontainer.Factory
}

func NewRuntime(path string) *Runtime {
	return &Runtime{
		ContainerConfigFactory: ContainerConfigFactory,

		root: path,
		s:    NewStorage(filepath.Join(path, storagePath)),
	}
}

func (r *Runtime) Init() error {
	var err error
	r.f, err = libcontainer.New(
		filepath.Join(r.root, containersPath),
		libcontainer.Cgroupfs,
		libcontainer.InitArgs(os.Args[0], "init"),
	)

	return err
}

func (r *Runtime) InstallDriver(d DriverImage, update bool) error {
	return r.s.Install(d, update)
}

func (r *Runtime) RemoveDriver(d DriverImage) error {
	return r.s.Remove(d)
}

func (r *Runtime) Command(d DriverImage, p *Process) (Command, error) {
	var err error
	cfg := r.ContainerConfigFactory()
	cfg.Rootfs, err = r.s.RootFS(d)
	if err != nil {
		return nil, err
	}

	c, err := r.f.Create(ulid.New().String(), cfg)
	if err != nil {
		return nil, err
	}

	return newCommand(c, p), nil
}

func ContainerConfigFactory() *configs.Config {
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV

	return &configs.Config{
		Capabilities: []string{
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FSETID",
			"CAP_FOWNER",
			"CAP_MKNOD",
			"CAP_NET_RAW",
			"CAP_SETGID",
			"CAP_SETUID",
			"CAP_SETFCAP",
			"CAP_SETPCAP",
			"CAP_NET_BIND_SERVICE",
			"CAP_SYS_CHROOT",
			"CAP_KILL",
			"CAP_AUDIT_WRITE",
		},
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWUTS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
			//{Type: configs.NEWUSER},
			{Type: configs.NEWNET},
		}),
		Cgroups: &configs.Cgroup{
			Name:   "test-container",
			Parent: "system",
			Resources: &configs.Resources{
				MemorySwappiness: nil,
				AllowAllDevices:  nil,
				AllowedDevices:   configs.DefaultAllowedDevices,
			},
		},
		MaskPaths: []string{
			"/proc/kcore",
			"/sys/firmware",
		},
		ReadonlyPaths: []string{
			"/proc/sys", "/proc/sysrq-trigger", "/proc/irq", "/proc/bus",
		},
		Devices:  configs.DefaultAutoCreatedDevices,
		Hostname: "testing",
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
				Data:        "newinstance,ptmxmode=0666,mode=0620,gid=5",
			},
			{
				Device:      "tmpfs",
				Source:      "shm",
				Destination: "/dev/shm",
				Data:        "mode=1777,size=65536k",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "mqueue",
				Destination: "/dev/mqueue",
				Device:      "mqueue",
				Flags:       defaultMountFlags,
			},
			{
				Source:      "sysfs",
				Destination: "/sys",
				Device:      "sysfs",
				Flags:       defaultMountFlags | syscall.MS_RDONLY,
			},
		},

		Networks: []*configs.Network{
			{
				Type:    "loopback",
				Address: "127.0.0.1/0",
				Gateway: "localhost",
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

func Bootstrap() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			panic(err)
		}
		panic("--this line should have never been executed, congratulations--")
	}
}
