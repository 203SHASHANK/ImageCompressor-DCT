.PHONY: build build-cli test test-race bench fuzz cover run lint format setup clean \
        docker-build docker-run docker-up docker-down docker-logs

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X imagecompressor-dct/internal/handlers.Version=$(VERSION)

build:
	go build -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server

build-cli:
	go build -ldflags="-s -w" -o bin/dctpress ./cmd/dctpress

test:
	go test -cover ./...

test-race:
	go test -race ./...

bench:
	go test -run '^$$' -bench . -benchmem ./internal/...

# Fuzz the decoders for a short while; raise -fuzztime for a longer run.
fuzz:
	go test -run '^$$' -fuzz FuzzDecode -fuzztime 30s ./internal/core/huffman/
	go test -run '^$$' -fuzz FuzzDecode -fuzztime 30s ./internal/core/dct/

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

run:
	go run ./cmd/server --port 8081

lint:
	golangci-lint run

format:
	gofmt -w -s .

# Python packages are only needed for the Pillow and OpenCV benchmark rows.
setup:
	go mod download
	pip3 install pillow opencv-python-headless numpy

clean:
	rm -rf bin/ coverage.out coverage.html

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t imagecompressor-dct:latest .

docker-run:
	docker run --rm -p 8080:8080 --name imagecompressor-dct imagecompressor-dct:latest

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker logs -f imagecompressor-dct
