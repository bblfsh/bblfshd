# bblfshd [![Build Status](https://travis-ci.org/bblfsh/bblfshd.svg?branch=master)](https://travis-ci.org/bblfsh/bblfshd) [![codecov](https://codecov.io/gh/bblfsh/bblfshd/branch/master/graph/badge.svg)](https://codecov.io/gh/bblfsh/bblfshd) [![license](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](https://github.com/bblfsh/bblfshd/blob/master/LICENSE) [![GitHub release](https://img.shields.io/github/release/bblfsh/bblfshd.svg)](https://github.com/bblfsh/bblfshd/releases)

This repository contains bblfsh daemon (*bblfshd*), which includes the
runtime that runs the driver in *containers* and the bblfshctl, a cli tool used
to control the installed drivers and query the status of the daemon.

Drivers are implemented as Docker images, each having their own repository in the 
[`bblfsh` organization](https://github.com/search?q=topic%3Adriver+org%3Abblfsh&type=Repositories)
on GitHub. For more information, see [bblfsh SDK documentation](https://doc.bblf.sh/writing-a-driver/babelfish-sdk.html).

## Getting Started

See the [Getting Started](https://doc.bblf.sh/using-babelfish/getting-started.html) guide.

### Quick start

This project is now part of [source{d} Engine](https://sourced.tech/engine),
which provides the simplest way to get started with a single command.
Visit [sourced.tech/engine](https://sourced.tech/engine) for more information.

#### Rootless mode

The recommended way to run *bblfshd* by itself is using Docker:

```sh
docker run --name bblfshd \
  -p 9432:9432 \
  -v /var/lib/bblfshd:/var/lib/bblfshd \
  -v /proc:/newproc \
  --security-opt seccomp=./bblfshd-seccomp.json \
  bblfshd
```

On macOS, use this command instead to use a Docker volume:

```sh
docker run --name bblfshd \
  -p 9432:9432 \
  -v bblfsh-storage:/var/lib/bblfshd bblfsh/bblfshd \
  -v /proc:/newproc \
  --security-opt seccomp=./bblfshd-seccomp.json \
  bblfshd
```


To understand the flags `-v /proc:/newproc` and `--security-opt seccomp=./bblfshd-seccomp.json`, 
where [`bblfshd-seccomp.json`](./bblfshd-seccomp.json) is a file present in this repo, and check 
further requirements, please refer to [rootless.md](./rootless.md). `bblfshd` is based on 
[container technology](https://github.com/opencontainers/runc/tree/master/libcontainer)
and interacts with the kernel at a low level. It exposes a gRPC server at the port `9432` by default 
which is used by the [clients](https://github.com/search?q=topic%3Aclient+org%3Abblfsh&type=Repositories)
to interact with the server. Also, we mount the path `/var/lib/bblfshd/` where
all the driver images and container instances will be stored.

#### Privileged mode

We advise against it, but if you prefer to run `bblfshd` in `privileged` mode to skip configuration steps of 
[rootless.md](rootless.md), you could do, in Linux:

```sh
docker run -d --name bblfshd --privileged -p 9432:9432 -v /var/lib/bblfshd:/var/lib/bblfshd bblfsh/bblfshd
```

or macOs:

```sh
docker run -d --name bblfshd --privileged -p 9432:9432 -v bblfsh-storage:/var/lib/bblfshd bblfsh/bblfshd
```

#### Install drivers

Now you need to install the driver images into the daemon, you can install
the official images just running the command:

```sh
docker exec -it bblfshd bblfshctl driver install --all
```

You can check the installed versions by executing:
```
docker exec -it bblfshd bblfshctl driver list
```

```
+----------+-------------------------------+---------+--------+---------+-----+-------------+
| LANGUAGE |             IMAGE             | VERSION | STATUS | CREATED | GO  |   NATIVE    |
+----------+-------------------------------+---------+--------+---------+-----+-------------+
| python   | //bblfsh/python-driver:latest | v1.1.5  | beta   | 4 days  | 1.8 | 3.6.2       |
| java     | //bblfsh/java-driver:latest   | v1.1.0  | alpha  | 6 days  | 1.8 | 8.131.11-r2 |
+----------+-------------------------------+---------+--------+---------+-----+-------------+
```

To test the driver you can execute a parse request to the server with the `bblfshctl parse` command,
and an example contained in the Docker image:

```sh
docker exec -it bblfshd bblfshctl parse /opt/bblfsh/etc/examples/python.py
```

## SELinux

If your system has SELinux enabled (which is the default in Fedora, Red Hat, CentOS
and many others) you'll need to compile and load a policy module before running the
bblfshd Docker image or running driver containers will fail with a `permission
denied` message in the logs. 

To do this, run these commands from the project root:

```bash
cd selinux/
sh compile.sh
semodule -i bblfshd.pp
```

If you were already running an instance of *bblfshd*, you will need to delete the
container (`docker rm -f bblfshd`) and run it again (`docker run...`).

Once the module has been loaded with `semodule` the change should persist even
if you reboot. If you want to permanently remove this module run `semodule -d bblfshd`.

Alternatively, you could set SELinux to permissive module with:

```
echo 1 > /sys/fs/selinux/enforce
```

(doing this on production systems which usually have SELinux enabled by default
should be strongly discouraged).

## Development

If you wish to work on *bblfshd* , you'll first need [Go](http://www.golang.org)
installed on your machine (version 1.11+ is *required*) and [Docker](https://docs.docker.com/engine/installation/).
Docker is used to build and run tests in an isolated environment.

For local development of *bblfshd*, first make sure Go is properly installed and
that a [GOPATH](http://golang.org/doc/code.html#GOPATH) has been set. You will
 also need to add `$GOPATH/bin` to your `$PATH`.

Next, using [Git](https://git-scm.com/), clone this repository into
`$GOPATH/src/github.com/bblfsh/bblfshd`. All the necessary dependencies are
automatically installed, so you just need to type `make`. This will compile the
code and then run the tests. If this exits with exit status 0, then everything
is working!


### Dependencies

Ensure you have [`ostree`](https://github.com/ostreedev/ostree) and development libraries for `ostree` installed.

You can install from your distribution pack manager as follow, or built [from source](https://github.com/ostreedev/ostree) (more on that [here](https://ostree.readthedocs.io/en/latest/#building)).

Debian, Ubuntu, and related distributions:
```
$ apt-get install libostree-dev
```

Fedora, CentOS, RHEL, and related distributions:
```bash
$ yum install -y ostree-devel
```

Arch and related distributions:

```bash
$ pacman -S ostree
```

### Building From Source

Build with:

```
$ make build
```

### Running Tests

Run tests with:

```
$ make test
```

### Environment variables

- `BBLFSHD_MAX_DRIVER_INSTANCES` - maximal number of driver instances for each language.
  Default to a number of CPUs.

- `BBLFSHD_MIN_DRIVER_INSTANCES` - minimal number of driver instances that will be run
  for each language. Default to 1.

### Enable tracing

Bblfshd supports [OpenTracing](https://opentracing.io/) that can be used to profile request on a high level or trace
individual requests to bblfshd and/or language drivers.

To enable it, you can use [Jaeger](https://www.jaegertracing.io/docs/1.8/getting-started/).
The easiest way is to start all-in-one Jaeger image:
```
docker run -d --name jaeger \
  -e COLLECTOR_ZIPKIN_HTTP_PORT=9411 \
  -p 5775:5775/udp \
  -p 6831:6831/udp \
  -p 6832:6832/udp \
  -p 5778:5778 \
  -p 16686:16686 \
  -p 14268:14268 \
  -p 9411:9411 \
  jaegertracing/all-in-one:1.8
```

For Docker installation of bblfshd add the following flags:
```
--link jaeger:jaeger -e JAEGER_AGENT_HOST=jaeger -e JAEGER_AGENT_PORT=6831 -e JAEGER_SAMPLER_TYPE=const -e JAEGER_SAMPLER_PARAM=1
```

For bblfshd running locally, set following environment variables:
```
JAEGER_AGENT_HOST=localhost JAEGER_AGENT_PORT=6831 JAEGER_SAMPLER_TYPE=const JAEGER_SAMPLER_PARAM=1
```

Run few requests, and check traces at http://localhost:16686.

For enabling tracing in production, consult [Jaeger documentation](https://www.jaegertracing.io/docs/1.8).

## License

GPLv3, see [LICENSE](LICENSE)

