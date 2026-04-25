# ── Stage 1: Build Go binary ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Fetch dependencies before copying source so this layer is cached
# and only re-runs when go.mod / go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -trimpath \
    -o /build/imagecompressor \
    ./cmd/server/main.go

# ── Stage 2: Runtime image ────────────────────────────────────────────────────
FROM python:3.11-slim

WORKDIR /app

# Install Python benchmark deps.
# Pinned for reproducibility; opencv-python-headless avoids GUI/X11 deps.
RUN pip install --no-cache-dir \
    pillow==10.4.0 \
    opencv-python-headless==4.9.0.80 \
    scikit-image==0.22.0 \
    numpy==1.26.4

# Copy statically-linked Go binary and runtime assets from the builder stage.
COPY --from=builder /build/imagecompressor ./
COPY web/                             ./web/
COPY internal/python/scripts/         ./internal/python/scripts/

# Run as non-root for container security.
RUN adduser --disabled-password --gecos '' appuser \
    && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080

# HEALTHCHECK uses the /health endpoint added to the server.
# --start-period gives Python a moment to warm up on first benchmark call.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python3 -c \
        "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" \
        || exit 1

CMD ["/app/imagecompressor"]
