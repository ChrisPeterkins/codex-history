VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build install

build:
	go build -ldflags "-X main.version=$(VERSION)" -o codex-history .

install: build
	sudo cp codex-history /usr/local/bin/codex-history
