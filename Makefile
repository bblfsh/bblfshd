all: clean test build

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
GO_PKG = github.com/bblfsh/$(PROJECT)
COMMANDS = bblfshd bblfshctl
DEPENDENCIES = \
	golang.org/x/tools/cmd/cover
NOVENDOR_PACKAGES := $(shell go list ./... | grep -v '/vendor/')

# Environment
BASE_PATH := $(shell pwd)
VENDOR_PATH := $(BASE_PATH)/vendor
BUILD_PATH := $(BASE_PATH)/build
CMD_PATH := $(BASE_PATH)/cmd

# Build information
BUILD ?= $(shell date +%FT%H:%M:%S%z)
GIT_COMMIT=$(shell git rev-parse HEAD | cut -c1-7)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "-dirty" || true)
DEV_PREFIX := dev
VERSION ?= $(DEV_PREFIX)-$(GIT_COMMIT)$(GIT_DIRTY)


# Go parameters
GO_CMD = go
GO_BUILD = $(GO_CMD) build
GO_GET = $(GO_CMD) get -v
GO_TEST = $(GO_CMD) test -v

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
DOCKER_BUILD = $(DOCKER_CMD) build --build-arg BBLFSHD_VERSION=$(VERSION) --build-arg BBLFSHD_BUILD="$(BUILD)"
DOCKER_RUN = $(DOCKER_CMD) run --rm
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
dependencies: build-fixture

docker-build:
	docker build --target=builder -t $(DOCKER_BUILD_IMAGE) .

test: dependencies docker-build
	$(DOCKER_RUN) --privileged  -v /var/run/docker.sock:/var/run/docker.sock -v $(GOPATH)/src/$(GO_PKG):/go/src/$(GO_PKG) -e TEST_NETWORKING=1 $(DOCKER_BUILD_IMAGE) $(GO_TEST) ./...

test-coverage: dependencies docker-build
	mkdir -p reports
	$(DOCKER_RUN) --privileged -v /var/run/docker.sock:/var/run/docker.sock -v $(GOPATH)/src/$(GO_PKG)/reports:/go/src/$(GO_PKG)/reports $(DOCKER_BUILD_IMAGE) make test-coverage-internal
	mv ./reports/* ./ && rm -rf reports

test-coverage-internal:
	export TEST_NETWORKING=1; \
	echo "" > reports/$(COVERAGE_REPORT); \
	for dir in $(NOVENDOR_PACKAGES); do \
		$(GO_TEST) $$dir -coverprofile=reports/$(COVERAGE_PROFILE) -covermode=$(COVERAGE_MODE); \
		if [ $$? != 0 ]; then \
			exit 2; \
		fi; \
		if [ -f reports/$(COVERAGE_PROFILE) ]; then \
			cat reports/$(COVERAGE_PROFILE) >> reports/$(COVERAGE_REPORT); \
			rm reports/$(COVERAGE_PROFILE); \
		fi; \
	done;

build: dependencies docker-build
	$(DOCKER_BUILD) -t $(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED)) .

build-fixture:
	cd $(BASE_PATH)/runtime/fixture/; \
	docker build -t $(DOCKER_IMAGE_FIXTURE) .


build-drivers: build
	docker build -f Dockerfile.drivers --build-arg TAG="$(VERSION)" -t "$(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED))-drivers" .

clean:
	rm -rf $(BUILD_PATH); \
	$(GO_CLEAN) .

push: build
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

push-drivers: build-drivers
	$(if $(pushdisabled),$(error $(pushdisabled)))

	@if [ "$$DOCKER_USERNAME" != "" ]; then \
		$(DOCKER_CMD) login -u="$$DOCKER_USERNAME" -p="$$DOCKER_PASSWORD"; \
	fi;

	$(DOCKER_PUSH) "$(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED))-drivers"
	@if [ "$$TRAVIS_TAG" != "" ]; then \
		$(DOCKER_TAG) "$(call unescape_docker_tag,$(DOCKER_IMAGE_VERSIONED))-drivers" \
			$(call unescape_docker_tag,$(DOCKER_IMAGE)):latest-drivers; \
		$(DOCKER_PUSH) $(call unescape_docker_tag,$(DOCKER_IMAGE):latest-drivers); \
	fi;

packages: dependencies docker-build
	$(DOCKER_RUN) -v $(BUILD_PATH):/go/src/$(GO_PKG)/build \
	                -e TRAVIS_BRANCH=$(TRAVIS_BRANCH) \
	                -e TRAVIS_TAG=$(TRAVIS_TAG) \
	                -e TRAVIS_PULL_REQUEST=$(TRAVIS_PULL_REQUEST) \
	                $(DOCKER_BUILD_IMAGE) make packages-internal

packages-internal: $(COMMANDS)

$(COMMANDS):
	for arch in $(PKG_ARCH); do \
		for os in $(PKG_OS_$@); do \
			mkdir -p $(BUILD_PATH)/$@_$${os}_$${arch}; \
			echo ; \
			echo "$${os} - $@"; \
			GOOS=$${os} GOARCH=$${arch}  $(GO_BUILD) $$([ $${os} = "linux" ] && echo -tags ostree) \
				--ldflags "$(LDFLAGS)" \
				-o "$(BUILD_PATH)/$@_$${os}_$${arch}/$@" \
				$(CMD_PATH)/$@/main.go; \
			cd $(BUILD_PATH); \
			tar -cvzf $@_$(VERSION)_$${os}_$${arch}.tar.gz $@_$${os}_$${arch}/; \
			cd $(BASE_PATH); \
		done; \
	done; \
