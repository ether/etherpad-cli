.PHONY: build test lint install clean

build:
	go build -o bin/etherpad-pp-cli ./cmd/etherpad-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/etherpad-pp-cli

clean:
	rm -rf bin/
