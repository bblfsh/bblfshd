# bblfshd [![Build Status](https://travis-ci.org/bblfsh/bblfshd.svg?branch=master)](https://travis-ci.org/bblfsh/bblfshd) [![codecov](https://codecov.io/gh/bblfsh/bblfshd/branch/master/graph/badge.svg)](https://codecov.io/gh/bblfsh/bblfshd) [![license](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](https://github.com/bblfsh/bblfshd/blob/master/LICENSE) [![GitHub release](https://img.shields.io/github/release/bblfsh/bblfshd.svg)](https://github.com/bblfsh/bblfshd/releases)

This repository contains bblfsh daemon (*bblfshd*), which includes the
runtime that runs the driver in *containers* and the bblfshctl, a cli tool used
too control the installed drivers and query the status of the daemon.

Drivers are implemented as docker image that each have their own repository in
[the `bblfsh` organization](https://github.com/search?q=topic%3Adriver+org%3Abblfsh&type=Repositories)
on GitHub. For more information, see [bblfsh SDK documentation](https://doc.bblf.sh/writing-a-driver/babelfish-sdk.html).

## Getting Started

See the [Getting Started](https://doc.bblf.sh/using-babelfish/getting-started.html) guide.

### Quick start

The recommended way to run *bblfshd* is using *docker*:

```sh
docker run -d --name bblfshd --privileged -p 9432:9432 -v /var/lib/bblfshd:/var/lib/bblfshd bblfsh/bblfshd
```

On macOS, use this command instead to use a docker volume:

```sh
docker run -d --name bblfshd --privileged -p 9432:9432 -v bblfsh-storage:/var/lib/bblfshd bblfsh/bblfshd
```


The container should be executed with the `--privileged` flag since *bblfshd* it's
based on [container technology](https://github.com/opencontainers/runc/tree/master/libcontainer)
and interacts with the kernel at a low level. *bblfshd*, expose a gRPC server at
the port `9432` by default, this gRPC will be used by the [clients](https://github.com/search?q=topic%3Aclient+org%3Abblfsh&type=Repositories)
to interact with the server. Also, we mount the path `/var/lib/bblfshd/` where
all the driver images and container instances will be stored.

Now you need to install the driver images into the daemon, you can install
the official images just running the command:

```sh
docker exec -it bblfshd bblfshctl driver install --all
```

You can check the installed versions executing:
```
docker exec -it bblfshd bblfshctl driver list
```

```
+----------+-------------------------------+---------+--------+---------+--------+-----+-------------+
| LANGUAGE |             IMAGE             | VERSION | STATUS | CREATED |   OS   | GO  |   NATIVE    |
+----------+-------------------------------+---------+--------+---------+--------+-----+-------------+
| python   | //bblfsh/python-driver:latest | v1.1.5  | beta   | 4 days  | alpine | 1.8 | 3.6.2       |
| java     | //bblfsh/java-driver:latest   | v1.1.0  | alpha  | 6 days  | alpine | 1.8 | 8.131.11-r2 |
+----------+-------------------------------+---------+--------+---------+--------+-----+-------------+
```

To test the driver you can executed a parse request to the server with the `bblfshctl parse` command,
and an example contained in the docker image:

```sh
docker exec -it bblfshd bblfshctl parse /opt/bblfsh/etc/examples/python.py
```

## SELinux

If your system has SELinux enabled (is the default in Fedora, Red Hat, CentOS
and many others) you need to compile and load a policy module before running the
bblfshd Docker image or running driver containers will fail with a `permission
denied` message in the logs. 

To do this, run these commands from the project root:

```bash
cd selinux/
sh compile.sh
semodule -i bblfshd.pp
```

If you were already running an instance of bblfshd, you will need to delete the
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
installed on your machine (version 1.10+ is *required*) and [Docker](https://docs.docker.com/engine/installation/),
docker is used to build and run the test in an isolated environment.

For local development of bblfshd, first make sure Go is properly installed and
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

## License

GPLv3, see [LICENSE](LICENSE)

