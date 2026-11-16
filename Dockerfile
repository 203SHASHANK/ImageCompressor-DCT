# Build stage: compile a static binary.
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy the module files first so the dependency download is cached until
# go.mod or go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X imagecompressor-dct/internal/handlers.Version=${VERSION}" \
    -trimpath \
    -o /build/imagecompressor \
    ./cmd/server

# Runtime stage: Python is here only for the Pillow and OpenCV benchmark rows.
FROM python:3.11-slim

WORKDIR /app

# opencv-python-headless avoids pulling in GUI libraries.
RUN pip install --no-cache-dir \
    pillow==10.4.0 \
    opencv-python-headless==4.9.0.80 \
    numpy==1.26.4

COPY --from=builder /build/imagecompressor ./
COPY web/ ./web/
COPY internal/python/scripts/ ./internal/python/scripts/

# Do not run as root.
RUN adduser --disabled-password --gecos '' appuser \
    && chown -R appuser:appuser /app
USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python3 -c \
        "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" \
        || exit 1

CMD ["/app/imagecompressor"]
