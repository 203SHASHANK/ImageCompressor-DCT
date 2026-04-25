.PHONY: build test run restart clean benchmark lint format setup \
        docker-build docker-run docker-up docker-down docker-logs

build:
	go build -o bin/server cmd/server/main.go

build-prod:
	go build -ldflags="-s -w" -o bin/server cmd/server/main.go

test:
	go test -v -cover ./...

test-race:
	go test -race ./...

run:
	go run cmd/server/main.go --port 8081

restart:
	-fuser -k 8081/tcp 2>/dev/null || true
	$(MAKE) build
	./bin/server --port 8081

benchmark:
	go test -bench=. -benchmem ./test/benchmarks/

clean:
	rm -rf bin/ tmp/

lint:
	golangci-lint run

format:
	gofmt -w -s .

coverage:
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

setup:
	go mod tidy
	pip3 install Pillow opencv-python scikit-image numpy 2>/dev/null || true

# ── Docker ────────────────────────────────────────────────────────────────────

docker-build:
	docker build -t imagecompressor-dct:latest .

docker-run:
	docker run --rm -p 8080:8080 --name imagecompressor-dct imagecompressor-dct:latest

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker logs -f imagecompressor-dct
