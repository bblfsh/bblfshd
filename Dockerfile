FROM alpine:3.6

RUN apk add --no-cache device-mapper ca-certificates
ADD cmd/test/test test
ENTRYPOINT ./test