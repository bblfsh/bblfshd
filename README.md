# bblfshd [![Build Status](https://travis-ci.org/bblfsh/bblfshd.svg?branch=master)](https://travis-ci.org/bblfsh/bblfshd) [![codecov](https://codecov.io/gh/bblfsh/bblfshd/branch/master/graph/badge.svg)](https://codecov.io/gh/bblfsh/bblfshd) [![license](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](https://github.com/bblfsh/bblfshd/blob/master/LICENSE) [![GitHub release](https://img.shields.io/github/release/bblfsh/bblfshd.svg)](Release)

This repository contains bblfsh daemon (*bblfshd*), which includes the
runtime that runs the driver in *containers* and the bblfshctl, a cli tool used
too control the installed drivers and query the status of the daemon.

Drivers are implemented as docker image that each have their own repository in
[the `bblfsh` organization](https://github.com/search?q=topic%3Adriver+org%3Abblfsh&type=Repositories)
on GitHub. For more information, see [bblfsh SDK documentation](https://doc.bblf.sh/driver/sdk.html).

## Getting Started

See the [Getting Started](https://doc.bblf.sh/user/getting-started.html) guide.

## Development

If you wish to work on *bblfshd* , you'll first need [Go](http://www.golang.org)
installed on your machine (version 1.9+ is *required*) and [Docker](https://docs.docker.com/engine/installation/),
docker its used to build and run the test in an isolated environment.

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

