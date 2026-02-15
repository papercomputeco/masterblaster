BINARY := mb
VERSION := 0.2.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test install fmt vet

build:
	go build $(LDFLAGS) -o ./build/$(BINARY) .

install: build
	cp ./build/$(BINARY) /usr/local/bin/

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf ./build/
