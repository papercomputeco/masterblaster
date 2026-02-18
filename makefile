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
build: ## Builds all cross-platform release artifacts via Dagger
	dagger call \
		build-release \
			--version $(VERSION) \
			--commit $(COMMIT) \
		export \
			--path ./build

.PHONY: build-local
build-local: ## Builds local artifacts with local toolchain
	$(call print-target)
	@mkdir -p ./build
	go build -ldflags "$(LDFLAGS)" -o ./build/$(BIN_NAME) .

install: build-local ## Builds local artifacts and installs to configured GOBIN
	$(call print-target)
	cp ./build/$(BIN_NAME) $(shell go env GOBIN)

.PHONY: upload-install-script
upload-install-script: ## Uploads the install script
	dagger call \
		upload-install-sh \
			--endpoint=env://BUCKET_ENDPOINT \
			--bucket=env://BUCKET_NAME \
			--access-key-id=env://BUCKET_ACCESS_KEY_ID \
			--secret-access-key=env://BUCKET_SECRET_ACCESS_KEY

.PHONY: release
release: ## Builds and releases mb artifacts
	dagger call \
		release-latest \
			--version=${VERSION} \
			--commit=${COMMIT} \
			--endpoint=env://BUCKET_ENDPOINT \
			--bucket=env://BUCKET_NAME \
			--access-key-id=env://BUCKET_ACCESS_KEY_ID \
			--secret-access-key=env://BUCKET_SECRET_ACCESS_KEY

check:
	dagger check

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
