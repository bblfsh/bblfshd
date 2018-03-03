# Docsrv: configure the languages whose api-doc can be auto generated
LANGUAGES = "go"
# Docs: do not edit this
DOCS_REPOSITORY := https://github.com/src-d/docs
SHARED_PATH ?= $(shell pwd)/.docsrv-resources
DOCS_PATH ?= $(SHARED_PATH)/.docs
$(DOCS_PATH)/Makefile.inc:
	git clone --quiet --depth 1 $(DOCS_REPOSITORY) $(DOCS_PATH);
-include $(DOCS_PATH)/Makefile.inc

# Package configuration
PROJECT = bblfshd
COMMANDS = bblfshd bblfshctl
DEPENDENCIES = \
	golang.org/x/tools/cmd/cover
NOVENDOR_PACKAGES := $(shell go list ./... | grep -v '/vendor/')

# Environment
BASE_PATH := $(shell pwd)
VENDOR_PATH := $(BASE_PATH)/vendor
BUILD_PATH := $(BASE_PATH)/build
CMD_PATH := $(BASE_PATH)/cli

# Build information
BUILD ?= $(shell date -Iseconds)
GIT_COMMIT=$(shell git rev-parse HEAD | cut -c1-7)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "-dirty" || true)
DEV_PREFIX := dev
VERSION ?= $(DEV_PREFIX)-$(GIT_COMMIT)$(GIT_DIRTY)


# Go parameters
GO_CMD = go
GO_BUILD = $(GO_CMD) build
GO_CLEAN = $(GO_CMD) clean
GO_GET = $(GO_CMD) get -v
GO_TEST = $(GO_CMD) test -v
DEP ?= $(GOPATH)/bin/dep
DEP_VERSION = v0.4.1

# Packages content
PKG_OS_bblfshctl = darwin linux windows
PKG_OS_bblfshd = linux
PKG_ARCH = amd64

# Coverage
COVERAGE_REPORT = coverage.txt
COVERAGE_PROFILE = profile.out
COVERAGE_MODE = atomic

ifneq ($(origin TRAVIS_TAG), undefined)
	VERSION := $(TRAVIS_TAG)
endif

# Build
LDFLAGS = -X main.version=$(VERSION) -X main.build=$(BUILD)

# Docker
DOCKER_CMD = docker
DOCKER_BUILD = $(DOCKER_CMD) build
DOCKER_RUN = $(DOCKER_CMD) run --rm -e VERSION=$(VERSION) -e BUILD=$(BUILD)
DOCKER_BUILD_IMAGE = bblfshd-build
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

# if we are not in master, and it's not a tag the push is disabled
ifneq ($(TRAVIS_BRANCH), master)
	ifeq ($(TRAVIS_TAG), )
        pushdisabled = "push disabled for non-master branches"
	endif
endif

# if this is a pull request, the push is disabled
ifneq ($(TRAVIS_PULL_REQUEST), false)
        pushdisabled = "push disabled for pull-requests"
endif

DOCKER_IMAGE ?= bblfsh/bblfshd
DOCKER_IMAGE_VERSIONED ?= $(call escape_docker_tag,$(DOCKER_IMAGE):$(VERSION))
DOCKER_IMAGE_FIXTURE ?= $(DOCKER_IMAGE):fixture

# Rules
all: clean test build

dependencies: $(DEPENDENCIES) $(VENDOR_PATH) build-fixture

$(DEPENDENCIES):
	$(GO_GET) $@/... && \
	wget -qO $(DEP) https://github.com/golang/dep/releases/download/$(DEP_VERSION)/dep-linux-amd64 && \
	chmod +x $(DEP)

$(VENDOR_PATH):
	rm -rf vendor/github.com/Sirupsen/;

docker-build:
	$(DOCKER_BUILD) -f Dockerfile.build -t $(DOCKER_BUILD_IMAGE) .

test: dependencies docker-build
	$(DOCKER_RUN) --privileged  -v /var/run/docker.sock:/var/run/docker.sock -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make test-internal

test-internal:
	export TEST_NETWORKING=1; \
	$(GO_TEST) $(NOVENDOR_PACKAGES)

test-coverage: dependencies docker-build
	$(DOCKER_RUN) --privileged -v /var/run/docker.sock:/var/run/docker.sock -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make test-coverage-internal

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
		$(GO_BUILD) --ldflags '$(LDFLAGS)' -o $(BUILD_PATH)/bin/$${cmd} .; \
	done;

build-fixture:
	cd $(BASE_PATH)/runtime/fixture/; \
	$(DOCKER_BUILD) -t $(DOCKER_IMAGE_FIXTURE) .

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

	$(DOCKER_PUSH) $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED))
	@if [ "$$TRAVIS_TAG" != "" ]; then \
		$(DOCKER_TAG) $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED)) \
			$(call unescape_docker_tag,$(DOCKER_IMAGE)):latest; \
		$(DOCKER_PUSH) $(call unescape_docker_tag,$(DOCKER_IMAGE):latest); \
	fi;

packages: dependencies docker-build
	$(if $(pushdisabled),$(error $(pushdisabled)))
	$(DOCKER_RUN) -v $(GOPATH):/go $(DOCKER_BUILD_IMAGE) make packages-internal

packages-internal: $(COMMANDS)

$(COMMANDS):
	for arch in $(PKG_ARCH); do \
		for os in $(PKG_OS_$@); do \
			mkdir -p $(BUILD_PATH)/$@_$${os}_$${arch}; \
			GOOS=$${os} GOARCH=$${arch} $(GO_BUILD) \
				--ldflags '$(LDFLAGS)' \
				-o "$(BUILD_PATH)/$@_$${os}_$${arch}/$@" \
				$(CMD_PATH)/$@/main.go; \
			cd $(BUILD_PATH); \
			tar -cvzf $@_$(VERSION)_$${os}_$${arch}.tar.gz $@_$${os}_$${arch}/; \
			cd $(BASE_PATH); \
		done; \
	done; \
