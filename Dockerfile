FROM golang:1.26@sha256:11fd8f7f63db3b6fb198797042ba4c40a4a34dc83325d3328ca3bc4bb7726786 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BRANCH
ARG COMMIT
ARG DATE
ARG VERSION

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X github.com/prometheus/common/version.Version=${VERSION} \
    -X github.com/prometheus/common/version.Revision=${COMMIT} \
    -X github.com/prometheus/common/version.Branch=${BRANCH} \
    -X github.com/prometheus/common/version.BuildDate=${DATE}" \
    -o ovs-exporter .

FROM gcr.io/distroless/static-debian13:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

ARG COMMIT
ARG DATE
ARG VERSION

LABEL io.prometheus.image.variant="distroless"
LABEL org.opencontainers.image.authors="Christopher Barnes <github@barnes.biz>"
LABEL org.opencontainers.image.created=${DATE}
LABEL org.opencontainers.image.description="OTel-native Prometheus exporter for Open vSwitch (OVS)"
LABEL org.opencontainers.image.documentation="https://github.com/barnes-c/ovs-exporter"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.revision=${COMMIT}
LABEL org.opencontainers.image.source="https://github.com/barnes-c/ovs-exporter"
LABEL org.opencontainers.image.title="ovs-exporter"
LABEL org.opencontainers.image.url="https://github.com/barnes-c/ovs-exporter"
LABEL org.opencontainers.image.vendor="Christopher Barnes"
LABEL org.opencontainers.image.version=${VERSION}

COPY --from=builder /src/ovs-exporter /bin/ovs-exporter
COPY LICENSE /

EXPOSE      10054
ENTRYPOINT  [ "/bin/ovs-exporter" ]
