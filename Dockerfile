FROM debian:stretch-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends --no-install-suggests \
        ca-certificates \
        libostree-1-1 \
    && apt-get clean

ADD build /bin/
ENTRYPOINT ["/bin/bblfshd"]

