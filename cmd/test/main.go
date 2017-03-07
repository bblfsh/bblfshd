package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/bblfsh/server/runtime"
)

func init() {
	runtime.Bootstrap()
}

var wg sync.WaitGroup

func main() {
	s := runtime.NewRuntime("/tmp/runtime")
	if err := s.Init(); err != nil {
		panic(err)
	}

	p := &runtime.Process{
		Args:   []string{"/bin/cat", "/etc/os-release"},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		//Stdin:  os.Stdin,
	}

	ubuntu, _ := runtime.NewDriverImage("//ubuntu:latest")
	alpine, _ := runtime.NewDriverImage("//alpine:latest")

	fmt.Println(ubuntu, alpine)

	wg.Add(2)
	go run(s, ubuntu, p)
	go run(s, alpine, p)
	wg.Wait()
}

func run(s *runtime.Runtime, i runtime.DriverImage, p *runtime.Process) {
	if err := s.InstallDriver(i, false); err != nil {
		panic(err)
	}

	c, err := s.Container(i, p)
	if err != nil {
		panic(err)
	}

	if err := c.Run(); err != nil {
		panic(err)
	}

	wg.Done()
}
