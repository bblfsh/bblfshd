FROM debian:stretch-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends --no-install-suggests \
        ca-certificates \
        libostree-1-1 \
    && apt-get clean

ADD build/bin /opt/bblfsh/bin/
ADD etc /opt/bblfsh/etc/
ENV PATH="/opt/bblfsh/bin:${PATH}"

ENTRYPOINT ["bblfshd"]

