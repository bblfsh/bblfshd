# Package configuration
PROJECT = server
COMMANDS = test
DEPENDENCIES = golang.org/x/tools/cmd/cover

# Environment
BASE_PATH := $(shell pwd)
BUILD_PATH := $(BASE_PATH)/build
CMD_PATH := $(BASE_PATH)/cmd
SHA1 := $(shell git log --format='%H' -n 1 | cut -c1-10)
BUILD := $(shell date +"%m-%d-%Y_%H_%M_%S")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD)

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOGET = $(GOCMD) get -v
GOTEST = $(GOCMD) test -v

# Coverage
COVERAGE_REPORT = coverage.txt
COVERAGE_PROFILE = profile.out
COVERAGE_MODE = atomic

# Docker
DOCKERCMD = docker
DOCKERBUILD = $(DOCKERCMD) build
DOCKERRUN = $(DOCKERCMD) run --rm
DOCKER_BUILD_IMAGE = babelfish-build


ifneq ($(origin TRAVIS_TAG), undefined)
	BRANCH := $(TRAVIS_TAG)
endif

# Build
LDFLAGS = -X main.version=$(BRANCH) -X main.build=$(BUILD)

# Rules
all: clean build

dependencies:
	@$(GOGET) -t ./...; \
	for i in $(DEPENDENCIES); do $(GOGET) $$i; done

test: dependencies
	@$(GOTEST) ./...

test-coverage: dependencies
	@cd $(BASE_PATH); \
	echo "" > $(COVERAGE_REPORT); \
	for dir in `find . -name "*.go" | grep -o '.*/' | sort | uniq`; do \
		$(GOTEST) $$dir -coverprofile=$(COVERAGE_PROFILE) -covermode=$(COVERAGE_MODE); \
		if [ $$? != 0 ]; then \
			exit 2; \
		fi; \
		if [ -f $(COVERAGE_PROFILE) ]; then \
			cat $(COVERAGE_PROFILE) >> $(COVERAGE_REPORT); \
			rm $(COVERAGE_PROFILE); \
		fi; \
	done; \

build: dependencies
	@$(DOCKERBUILD) -f Dockerfile.build -t $(DOCKER_BUILD_IMAGE) .; \
	$(DOCKERRUN) -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make build-local

build-local:
	@cd $(BASE_PATH); \
	mkdir -p $(BUILD_PATH); \
	for cmd in $(COMMANDS); do \
        cd $(CMD_PATH)/$${cmd}; \
		$(GOCMD) build --ldflags '$(LDFLAGS)' -o $(BUILD_PATH)/$${cmd} .; \
	done; \

clean:
	@rm -rf $(BUILD_PATH); \
	$(GOCLEAN) .