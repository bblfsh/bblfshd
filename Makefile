# Package configuration
PROJECT = server
COMMANDS = bblfsh
DEPENDENCIES = \
	golang.org/x/tools/cmd/cover \
	github.com/Masterminds/glide
NOVENDOR_PACKAGES := $(shell go list ./... | grep -v '/vendor/')

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
GLIDE ?= $(GOPATH)/bin/glide

# Coverage
COVERAGE_REPORT = coverage.txt
COVERAGE_PROFILE = profile.out
COVERAGE_MODE = atomic

ifneq ($(origin TRAVIS_TAG), undefined)
	BRANCH := $(TRAVIS_TAG)
endif

# Build
LDFLAGS = -X main.version=$(BRANCH) -X main.build=$(BUILD)

# Docker
DOCKER_CMD = docker
DOCKER_BUILD = $(DOCKER_CMD) build
DOCKER_RUN = $(DOCKER_CMD) run --rm
DOCKER_BUILD_IMAGE = bblfsh-server-build
DOCKER_TAG ?= $(DOCKER_CMD) tag
DOCKER_PUSH ?= $(DOCKER_CMD) push

# escape_docker_tag escape colon char to allow use a docker tag as rule
define escape_docker_tag
$(subst :,--,$(1))
endef

# unescape_docker_tag an escaped docker tag to be use in a docker command
define unescape_docker_tag
$(subst --,:,$(1))
endef

DOCKER_DEV_PREFIX := dev
DOCKER_VERSION ?= $(DOCKER_DEV_PREFIX)-$(shell git rev-parse HEAD | cut -c1-7)

# if TRAVIS_TAG defined DOCKER_VERSION is overrided
ifneq ($(TRAVIS_TAG), )
    DOCKER_VERSION := $(TRAVIS_TAG)
endif

# if we are not in tag, the push is disabled
ifeq ($(firstword $(subst -, ,$(DOCKER_VERSION))), $(DOCKER_DEV_PREFIX))
        pushdisabled = "push disabled for development versions"
endif


DOCKER_IMAGE ?= bblfsh/server
DOCKER_IMAGE_VERSIONED ?= $(call escape_docker_tag,$(DOCKER_IMAGE):$(DOCKER_VERSION))

# Rules
all: clean build

dependencies: $(DEPENDENCIES) $(VENDOR_PATH) $(NOVENDOR_PACKAGES)

$(DEPENDENCIES):
	$(GO_GET) $@/...

$(NOVENDOR_PACKAGES):
	$(GO_GET) $@

$(VENDOR_PATH):
	$(GLIDE) install

docker-build:
	$(DOCKER_BUILD) -f Dockerfile.build -t $(DOCKER_BUILD_IMAGE) .

test: dependencies docker-build
	$(DOCKER_RUN) --privileged -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make test-internal

test-internal:
	export TEST_NETWORKING=1; \
	$(GO_TEST) $(NOVENDOR_PACKAGES)

test-coverage: dependencies docker-build
	$(DOCKER_RUN) --privileged -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make test-coverage-internal

test-coverage-internal:
	export TEST_NETWORKING=1; \
	echo "" > $(COVERAGE_REPORT); \
	for dir in $(NOVENDOR_PACKAGES); do \
		$(GO_TEST) $$dir -coverprofile=$(COVERAGE_PROFILE) -covermode=$(COVERAGE_MODE); \
		if [ $$? != 0 ]; then \
			exit 2; \
		fi; \
		if [ -f $(COVERAGE_PROFILE) ]; then \
			cat $(COVERAGE_PROFILE) >> $(COVERAGE_REPORT); \
			rm $(COVERAGE_PROFILE); \
		fi; \
	done;

build: dependencies docker-build
	$(DOCKER_RUN) -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make build-internal

build-internal:
	mkdir -p $(BUILD_PATH); \
	for cmd in $(COMMANDS); do \
        cd $(CMD_PATH)/$${cmd}; \
		$(GO_CMD) build --ldflags '$(LDFLAGS)' -o $(BUILD_PATH)/$${cmd} .; \
	done;

docker-image-build: build
	$(DOCKER_BUILD) -t $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED)) .

clean:
	rm -rf $(BUILD_PATH); \
	$(GO_CLEAN) .

push: docker-image-build
	$(if $(pushdisabled),$(error $(pushdisabled)))

	@if [ "$$DOCKER_USERNAME" != "" ]; then \
		$(DOCKER_CMD) login -u="$$DOCKER_USERNAME" -p="$$DOCKER_PASSWORD"; \
	fi;

	$(DOCKER_TAG) $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED)) \
		$(call unescape_docker_tag,$(DOCKER_IMAGE)):latest
	$(DOCKER_PUSH) $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED))
	$(DOCKER_PUSH) $(call unescape_docker_tag,$(DOCKER_IMAGE):latest)

