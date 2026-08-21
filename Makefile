.PHONY: test test-race vet build verify run

test:
	go test ./... -count=1

test-race:
	go test -race ./... -count=1

vet:
	go vet ./...

build:
	go build ./...

verify: test test-race vet build

run:
	go run ./cmd/server
