GO        ?= go
VERSION   := $(shell cat Version)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BRANCH    := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
DATE      := $(shell date -u +%Y%m%d-%H:%M:%S)
BUILDUSER := $(shell id -un)@$(shell hostname -s)

LDFLAGS := \
  -s -w \
  -X github.com/prometheus/common/version.Version=$(VERSION) \
  -X github.com/prometheus/common/version.Revision=$(COMMIT) \
  -X github.com/prometheus/common/version.Branch=$(BRANCH) \
  -X github.com/prometheus/common/version.BuildUser=$(BUILDUSER) \
  -X github.com/prometheus/common/version.BuildDate=$(DATE)

GORELEASER_CONFIG := .github/.goreleaser.yml

COMPOSE_FILE := examples/smoke/docker-compose.yml
COMPOSE      ?= podman compose

.PHONY: all build build-all test vet lint fmt tidy snapshot release check clean \
        smoke-up smoke-down smoke-logs

all: fmt vet lint build test

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o ovs-exporter .

build-all:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o ovs-exporter-linux-amd64 .
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o ovs-exporter-linux-arm64 .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run --config .github/.golangci.yml ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

snapshot:
	goreleaser release --snapshot --clean --config $(GORELEASER_CONFIG)

release:
	goreleaser release --clean --config $(GORELEASER_CONFIG)

check:
	goreleaser check --config $(GORELEASER_CONFIG)

clean:
	rm -rf dist/ ovs-exporter ovs-exporter-*

smoke-up:
	$(COMPOSE) -f $(COMPOSE_FILE) up --build -d

smoke-down:
	-$(COMPOSE) -f $(COMPOSE_FILE) down --volumes

smoke-logs:
	$(COMPOSE) -f $(COMPOSE_FILE) logs -f
