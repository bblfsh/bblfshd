package main

import (
	"os"
	"runtime"
	"sync"

	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/the-babelfish/server/core"
)

func init() {
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

var wg sync.WaitGroup

func main() {
	s := &core.Server{
		RootPath: "/tmp/bb",
	}

	if err := s.Init(); err != nil {
		panic(err)
	}

	wg.Add(2)
	go run(s, "/tmp/alpine")
	go run(s, "/tmp/ubuntu")

	wg.Wait()
}

func run(s *core.Server, rootfs string) {

	p := &libcontainer.Process{
		Args:   []string{"/bin/cat", "/etc/os-release"},
		Env:    []string{"PATH=/bin"},
		User:   "daemon",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	c, err := s.Command(core.GetConfig(rootfs), p)
	if err != nil {
		panic(err)
	}

	if err := c.Run(); err != nil {
		panic(err)
	}

	wg.Done()
}

/*
i, err := core.NewDriverImage("docker://alpine:latest")
if err != nil {
	panic(err)
}

i.WriteTo(rootfs)
*/
