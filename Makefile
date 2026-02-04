BINARY := mb
VERSION := 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test install

build:
	go build $(LDFLAGS) -o $(BINARY) .

install: build
	cp $(BINARY) /usr/local/bin/

test:
	go test ./...

clean:
	rm -f $(BINARY)
