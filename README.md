# server [![Build Status](https://travis-ci.org/bblfsh/server.svg?branch=master)](https://travis-ci.org/bblfsh/server) [![codecov](https://codecov.io/gh/bblfsh/server/branch/master/graph/badge.svg)](https://codecov.io/gh/bblfsh/server)

## Getting Started

See the [Getting Started](https://doc.bblf.sh/user/getting-started.html) guide.

## Development

Ensure you have [GOPATH set up](https://golang.org/doc/code.html#GOPATH) and
[Docker installed](https://www.docker.com/get-docker).

Make sure you the repo is located properly inside `GOPATH`:

```
mkdir -p $GOPATH/src/github.com/bblfsh
cd $GOPATH/src/github.com/bblfsh
git clone https://github.com/bblfsh/server.git
cd server
```

### Dependencies

Ensure you have [OSTree](https://github.com/ostreedev/ostree) installed.

For Debian/Ubuntu:

```
$ apt-get install libostree-dev
```

For ArchLinux:

```
$ pacman -S ostree
```

### Building From Source

Build with:

```
$ make dependencies
$ make build
```

### Running Tests

Run tests with:

```
$ make test
```

## License

GPLv3, see [LICENSE](LICENSE)

