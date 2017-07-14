FROM alpine:3.6

RUN apk add --no-cache device-mapper ca-certificates
ADD build/bblfsh /bin/bblfsh
ENTRYPOINT ["/bin/bblfsh", "server"]
