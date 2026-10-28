.PHONY: build test lint fmt

build:
	go build -o bin/overhead ./cmd/overhead

test:
	go test -race ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

fmt:
	gofmt -w .
