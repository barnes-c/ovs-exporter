FROM debian:trixie-slim

RUN apt-get update \
       && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       openvswitch-switch \
       openvswitch-common \
       iproute2 \
       procps \
       && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 6640

ENTRYPOINT ["/entrypoint.sh"]
