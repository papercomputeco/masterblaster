# Masterblaster makefile
#
# Based around the auto-documented Makefile:
# http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html

BIN_NAME := mb
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse HEAD)
BUILDTIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S')

LDFLAGS := -s -w \
	-X 'github.com/papercomputeco/masterblaster/pkg/utils.Version=$(VERSION)' \
	-X 'github.com/papercomputeco/masterblaster/pkg/utils.Sha=$(COMMIT)' \
	-X 'github.com/papercomputeco/masterblaster/pkg/utils.Buildtime=$(BUILDTIME)'

.PHONY: build
build: ## Builds binary
	go build -ldflags "$(LDFLAGS)" -o ./build/$(BIN_NAME) .

.PHONY: build-local
build-local: ## Builds local artifacts with local toolchain
	$(call print-target)
	@mkdir -p ./build
	go build -ldflags "$(LDFLAGS)" -o ./build/$(BIN_NAME) .

install: build-local ## Builds local artifacts and installs to configured GOBIN
	$(call print-target)
	cp ./build/$(BIN_NAME) $(shell go env GOBIN)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf ./build/

.PHONY: help
.DEFAULT_GOAL := help
help: ## Prints this help message
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

define print-target
    @printf "Executing target: \033[36m$@\033[0m\n"
endef
