.PHONY: run build test

run:
	go run ./cmd/domainry-delivery

build:
	mkdir -p bin
	go build -o bin/domainry-delivery ./cmd/domainry-delivery

test:
	go test ./...
