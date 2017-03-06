# Package configuration
PROJECT = server
COMMANDS = test
DEPENDENCIES = \
	golang.org/x/tools/cmd/cover \
	github.com/Masterminds/glide

# Environment
BASE_PATH := $(shell pwd)
VENDOR_PATH := $(BASE_PATH)/vendor
BUILD_PATH := $(BASE_PATH)/build
CMD_PATH := $(BASE_PATH)/cmd
SHA1 := $(shell git log --format='%H' -n 1 | cut -c1-10)
BUILD := $(shell date +"%m-%d-%Y_%H_%M_%S")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD)

# Go parameters
GO_CMD = go
GO_BUILD = $(GO_CMD) build
GO_CLEAN = $(GO_CMD) clean
GO_GET = $(GO_CMD) get -v
GO_TEST = $(GO_CMD) test -v
GLIDE = glide

# Coverage
COVERAGE_REPORT = coverage.txt
COVERAGE_PROFILE = profile.out
COVERAGE_MODE = atomic

# Docker
DOCKER_CMD = docker
DOCKER_BUILD = $(DOCKERCMD) build
DOCKER_RUN = $(DOCKERCMD) run --rm
DOCKER_BUILD_IMAGE = babelfish-build


ifneq ($(origin TRAVIS_TAG), undefined)
	BRANCH := $(TRAVIS_TAG)
endif

# Build
LDFLAGS = -X main.version=$(BRANCH) -X main.build=$(BUILD)

# Rules
all: clean build

dependencies: $(DEPENDENCIES) $(VENDOR_PATH)

$(DEPENDENCIES):
	$(GO_GET) $@/...

$(VENDOR_PATH):
	$(GLIDE) install

test: dependencies
	$(GO_TEST) $(shell $(GLIDE) novendor)

test-coverage: dependencies
	echo "" > $(COVERAGE_REPORT); \
	for dir in `$(GO_CMD) list ./... | egrep -v '/(vendor|etc)/'`; do \
		$(GO_TEST) $$dir -coverprofile=$(COVERAGE_PROFILE) -covermode=$(COVERAGE_MODE); \
		if [ $$? != 0 ]; then \
			exit 2; \
		fi; \
		if [ -f $(COVERAGE_PROFILE) ]; then \
			cat $(COVERAGE_PROFILE) >> $(COVERAGE_REPORT); \
			rm $(COVERAGE_PROFILE); \
		fi; \
	done;

build: dependencies
	$(DOCKER_BUILD) -f Dockerfile.build -t $(DOCKER_BUILD_IMAGE) .; \
	$(DOCKER_RUN) -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make build-internal

build-internal:
	mkdir -p $(BUILD_PATH); \
	for cmd in $(COMMANDS); do \
        cd $(CMD_PATH)/$${cmd}; \
		$(GO_CMD) build --ldflags '$(LDFLAGS)' -o $(BUILD_PATH)/$${cmd} .; \
	done;

clean:
	rm -rf $(BUILD_PATH); \
	$(GOCLEAN) .