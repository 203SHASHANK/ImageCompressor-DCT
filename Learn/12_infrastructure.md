# Phase 12 — Infrastructure: Docker, Makefile, and Deployment

> *The algorithms are the core, but deployment is the wrapper that makes them accessible. This phase covers the Dockerfile, Makefile, and go.mod — the scaffolding that builds, runs, and ships DCTPress.*

---

## `go.mod` — The Module Manifest

```
module imagecompressor-dct

go 1.25.0

require golang.org/x/image v0.39.0
```

DCTPress has **one external dependency**: `golang.org/x/image`. Everything else is Go standard library.

### Why So Few Dependencies?

The Go standard library includes:
- `net/http` — full HTTP server with TLS support
- `encoding/json` — JSON encoding/decoding
- `image`, `image/jpeg`, `image/png` — core image types and codecs
- `sync` — Mutex, WaitGroup, Once
- `container/heap` — min-heap (used by Huffman)
- `math` — trigonometric functions
- `os/exec` — subprocess execution
- `flag` — command-line parsing

The `golang.org/x/image` package fills the gaps: BMP, TIFF, and WebP decoders not in the stdlib.

Minimal dependencies mean:
- No supply chain risk (a compromised dependency can't affect this project)
- Fast builds (no large transitive dependency trees)
- Long-term stability (stdlib APIs change very rarely)

### `go.sum` — Dependency Checksums

```
golang.org/x/image v0.39.0 h1:...
golang.org/x/image v0.39.0/go.mod h1:...
```

`go.sum` contains cryptographic hashes of every module and its `go.mod`. When `go mod download` fetches dependencies, it verifies the hashes match. This prevents a dependency from being silently tampered with on the module proxy.

---

## Makefile — The Development Workflow

```makefile
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
```

### `build` vs `build-prod`

Regular build: `go build -o bin/server cmd/server/main.go`
- Includes debug symbols, function names, line number information
- Binary is larger (~12 MB) but enables stack traces with line numbers

Production build: `go build -ldflags="-s -w" -o bin/server cmd/server/main.go`
- `-s`: omit the symbol table (strips function/variable names)
- `-w`: omit DWARF debug information
- Result: binary ~30–40% smaller (~8 MB), no debug symbols
- Used in the Dockerfile for container deployment

### `restart` — Development Convenience

```makefile
restart:
    -fuser -k 8081/tcp 2>/dev/null || true
    $(MAKE) build
    ./bin/server --port 8081
```

`-fuser -k 8081/tcp` — kills whatever process holds port 8081 (the current server instance). The `-` before `fuser` tells Make to continue even if this command fails (it fails if nothing is on the port). `2>/dev/null` suppresses error output. `|| true` ensures the command always succeeds.

Then rebuild and start the new binary. One command to update a running development server.

### `setup` — Environment Bootstrap

```makefile
setup:
    go mod tidy
    pip3 install Pillow opencv-python scikit-image numpy 2>/dev/null || true
```

`go mod tidy` — downloads Go dependencies and removes unused ones. `pip3 install ...` installs the Python packages needed for benchmarking. `|| true` makes the pip install non-fatal (if Python isn't installed, setup still succeeds).

### Test Targets

| Target | Command | Purpose |
|--------|---------|---------|
| `test` | `go test -v -cover ./...` | All tests with verbose output and coverage |
| `test-race` | `go test -race ./...` | Tests with data race detection |
| `benchmark` | `go test -bench=. -benchmem ./test/benchmarks/` | Performance microbenchmarks |
| `coverage` | coverage.out + HTML report | Visual coverage report |

`./...` is Go's pattern for "all packages recursively". It finds every package in the module and runs its tests.

---

## Dockerfile — Multi-Stage Build

The Dockerfile uses a **multi-stage build** — a Docker feature where you use multiple `FROM` stages and copy artifacts between them.

### Stage 1: Go Builder

```dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache layer: copy module files first (changes rarely)
COPY go.mod go.sum ./
RUN go mod download

# Source layer: copy everything else (changes often)
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -trimpath \
    -o /build/imagecompressor \
    ./cmd/server/main.go
```

**Why `golang:1.25-alpine`?** Alpine Linux is a minimal Linux distribution (~5 MB base image). The Go toolchain adds ~300 MB, but this is only the build stage — it doesn't end up in the final image.

**Layer caching**: Docker caches each `RUN` layer. If `go.mod` and `go.sum` haven't changed, `go mod download` is cached. Only when Go source changes does the compile layer execute. This makes rebuilds fast in CI.

**`CGO_ENABLED=0`**: Disables C Go (CGO), which allows Go to call C code. With CGO disabled, the binary is **statically linked** — it has no runtime dependencies on C libraries. The final container can run a minimal base image without libc.

**`GOOS=linux`**: Compile for Linux even if building on macOS. Cross-compilation support is a first-class Go feature.

**`-trimpath`**: Removes absolute file paths from the binary. Without this, stack traces include the builder's filesystem paths (`/build/...`), which are different from the container's paths. With trimpath, paths are relative to the module root.

### Stage 2: Python Runtime

```dockerfile
FROM python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir \
    pillow==10.4.0 \
    opencv-python-headless==4.9.0.80 \
    scikit-image==0.22.0 \
    numpy==1.26.4

COPY --from=builder /build/imagecompressor ./
COPY web/                             ./web/
COPY internal/python/scripts/         ./internal/python/scripts/
```

**`python:3.11-slim`**: Python runtime without documentation and test files. ~150 MB. Provides both Python for the benchmark scripts and libc for the Go binary (but since CGO is disabled, libc isn't actually needed).

**Pinned Python dependencies**: Exact versions (`pillow==10.4.0`) ensure reproducible builds. Without pins, `pip install Pillow` might install a newer version that breaks the API (e.g., the `channel_axis` vs `multichannel` change in scikit-image).

**`opencv-python-headless`**: The `headless` variant omits GUI dependencies (Qt, X11 libraries). The server doesn't have a display; requiring GUI libraries would add 500+ MB to the image unnecessarily.

**`--no-cache-dir`**: Don't cache pip downloads in the image layer. Since this is a single Docker build step, the cache would just take space.

**`COPY --from=builder /build/imagecompressor ./`**: This is the multi-stage magic. Only the compiled binary is copied from the builder stage — not the Go toolchain, not the source code, not the intermediate object files. The builder stage's 300 MB Go toolchain is discarded.

### Security and Operations

```dockerfile
RUN adduser --disabled-password --gecos '' appuser \
    && chown -R appuser:appuser /app
USER appuser
```

**Non-root user**: Running as `appuser` limits damage if the server is compromised. Container processes should never run as root. `--disabled-password` creates a user that cannot login (no shell, no password).

```dockerfile
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python3 -c \
        "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" \
        || exit 1

CMD ["/app/imagecompressor"]
```

**`EXPOSE 8080`**: Documents that the container listens on port 8080 (doesn't actually publish it; that's `-p 8080:8080` at runtime).

**`HEALTHCHECK`**: Docker periodically runs this command. If it fails 3 consecutive times (`--retries=3`), the container is marked unhealthy. The `--start-period=10s` gives Python 10 seconds to warm up before health checks begin.

The health check uses Python's `urllib.request` to call the `/health` endpoint. This exercises both the Go server and the Python availability check inside `HealthHandler`.

**`CMD ["/app/imagecompressor"]`**: The command to run when the container starts. Array form (not string form) avoids shell interpretation — important when arguments contain spaces or special characters.

---

## `docker-compose.yml`

```yaml
services:
  imagecompressor:
    build: .
    ports:
      - "8080:8080"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "python3", "-c",
             "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')"]
      interval: 30s
      timeout: 5s
      retries: 3
```

`docker compose up` — builds and starts the container. The server is available at `http://localhost:8080`.

`restart: unless-stopped` — automatically restarts the container if it crashes (but not if explicitly stopped by the user).

---

## `.dockerignore`

Similar to `.gitignore`, `.dockerignore` lists files to exclude from the Docker build context. Without it, Docker would send the entire project directory (including `bin/`, test artifacts, `.git/`) to the Docker daemon. Key exclusions:

```
bin/
tmp/
.git/
*.log
```

This speeds up `docker build` by not transferring irrelevant files.

---

## How Everything Connects

```
Developer workflow:
    git clone → make setup → make run → browser at :8081

Docker workflow:
    docker compose up → browser at :8080

Makefile targets map to:
    make run       → go run cmd/server/main.go
    make build     → go build → bin/server
    make restart   → kill :8081, rebuild, start bin/server
    make test      → go test -v -cover ./...
    make docker-up → docker compose up -d

Binary behavior:
    ./bin/server            → listens on :8080 (env SERVER_PORT default)
    ./bin/server --port 8081 → listens on :8081
    SERVER_PORT=9000 ./bin/server → listens on :9000

Docker container:
    CMD ["/app/imagecompressor"] → listens on :8080
    Port mapped to host by docker compose
```

---

## Final Summary — The Full Picture

DCTPress is built from these interconnected layers:

```
╔════════════════════════════════════════╗
║            Infrastructure             ║
║  Dockerfile  |  Makefile  |  go.mod   ║
╠════════════════════════════════════════╣
║              HTTP Layer               ║
║     cmd/server/main.go (routes)       ║
║     internal/handlers/ (handlers)     ║
╠════════════════════════════════════════╣
║           Service Layer               ║
║   internal/services/ (orchestration)  ║
╠════════════════════════════════════════╣
║     Algorithm Layer (Pure Go)         ║
║  internal/core/dct/     (encoder)     ║
║  internal/core/dct/     (decoder)     ║
║  internal/core/huffman/ (entropy)     ║
║  internal/core/metrics/ (quality)     ║
╠════════════════════════════════════════╣
║          External Integration         ║
║  internal/python/ (subprocess bridge) ║
║  internal/io/ (file loading/writing)  ║
╠════════════════════════════════════════╣
║            Static Assets              ║
║  web/ (HTML, JS, CSS)                 ╠════════════════════════════════════════╣
║              Testing                  ║
║  test/unit/  test/integration/        ║
╚════════════════════════════════════════╝
```

The mathematical core (DCT → quantize → zigzag → Huffman) is isolated from HTTP concerns. The HTTP layer is isolated from Python subprocess concerns. Each layer can be developed, tested, and replaced independently.

---

*This concludes the Learn series. Return to [Phase 00 — Overview](00_overview_and_mental_model.md) for the big picture.*
