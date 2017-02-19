FROM alpine

RUN apk add --no-cache device-mapper ca-certificates
ADD cmd/test/test test
ENTRYPOINT ./test