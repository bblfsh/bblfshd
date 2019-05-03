# Base builder image with all libraries installed, including the source of the project
FROM golang:1.12 as builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libostree-dev \
        libglib2.0-dev \
        btrfs-tools \
    && apt-get clean

ENV GOPATH=/go
ENV GO111MODULE=on
WORKDIR /go/src/github.com/bblfsh/bblfshd

ADD . .


# Actual build image that compiles bblfshd and bblfshctl
FROM builder as binbuild

RUN mkdir /build

ARG BBLFSHD_VERSION=dev
ARG BBLFSHD_BUILD=unknown

ENV GO_LDFLAGS="-X 'main.version=${BBLFSHD_VERSION}' -X 'main.build=${BBLFSHD_BUILD}'"

RUN go build  -tags ostree --ldflags "${GO_LDFLAGS}" -o /build/bblfshd ./cmd/bblfshd/
RUN go build --ldflags "${GO_LDFLAGS}" -o /build/bblfshctl ./cmd/bblfshctl/


# Final image for bblfshd
FROM debian:stretch-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends --no-install-suggests \
        ca-certificates \
        libostree-1-1 \
    && apt-get clean

ENV TINI_VERSION v0.18.0
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini /tini
RUN chmod +x /tini

COPY --from=binbuild build /opt/bblfsh/bin/
ADD etc /opt/bblfsh/etc/
ENV PATH="/opt/bblfsh/bin:${PATH}"

# Run bblfshd under Tini (see https://github.com/krallin/tini/issues/8 for details)
ENTRYPOINT ["/tini", "--", "bblfshd"]

