FROM alpine:3.6

RUN apk add --no-cache device-mapper ca-certificates
ADD build /bin/
ENTRYPOINT ["/bin/bblfshd"]
