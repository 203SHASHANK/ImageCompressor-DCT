# DCTPress — Complete Learning Guide

> *A comprehensive, bottom-up explanation of every concept, algorithm, file, and design decision in DCTPress — the from-scratch DCT image codec built in Go.*

---

## How to Read This

These documents are designed to be read in order. Each phase builds on the previous. If you already know Go, skip Phase 01. If you know image fundamentals, skip Phase 02.

Every code block in these documents is taken directly from the actual source files — nothing is invented or simplified. Every claim about performance numbers comes from real benchmark runs.

---

## Phase Index

| Phase | Title | Key Concepts |
|-------|-------|-------------|
| [00](00_overview_and_mental_model.md) | Overview & Mental Model | Big picture, file map, pipeline diagram |
| [01](01_go_language_and_module_system.md) | Go Language & Module System | packages, goroutines, interfaces, channels, error handling |
| [02](02_image_fundamentals_and_color_spaces.md) | Image Fundamentals & Color Spaces | RGB, YCbCr, chroma subsampling, 8×8 blocks |
| [03](03_dct_algorithm.md) | The DCT Algorithm | DCT-II formula, separability, precomputed cosines, energy compaction |
| [04](04_quantization_and_human_perception.md) | Quantization & Human Perception | ISO Annex K tables, quality scaling, HVS sensitivity, DC tuning |
| [05](05_encode_pipeline.md) | The Full Encode Pipeline | Worker pool, zigzag, DC delta, EOB, binary packing |
| [06](06_decode_pipeline.md) | The Full Decode Pipeline | Header parsing, DC reconstruction, bilinear upsampling |
| [07](07_huffman_entropy_coding.md) | Huffman Entropy Coding | Min-heap tree building, prefix codes, bit packing, adaptive per-image |
| [08](08_quality_metrics.md) | Quality Metrics: PSNR & SSIM | MSE, PSNR dB scale, SSIM perceptual model, window statistics |
| [09](09_http_api_and_services.md) | HTTP API & Service Layer | REST endpoints, in-memory session state, service orchestration |
| [10](10_python_bridge_and_benchmarking.md) | Python Bridge & Benchmarking | subprocess pattern, JSON IPC, PIL vs OpenCV vs DCTPress |
| [11](11_testing_strategy.md) | Testing Strategy | Unit tests, round-trip quality tests, integration tests, benchmarks |
| [12](12_infrastructure.md) | Infrastructure | Dockerfile multi-stage, Makefile targets, go.mod, docker-compose |

---

## Quick Reference — Core Algorithms

### DCT Formula (1D, length 8)
```
X[k] = α(k)/2 · Σₙ x[n] · cos((2n+1)·k·π/16)
α(k) = 1/√2 for k=0, else 1
```

### Quality Scale Factor
```
if quality < 50: scale = 50 / quality
else:            scale = 2 - quality/50
```

### PSNR
```
PSNR = 10 · log₁₀(255² / MSE)
```

### SSIM (per window)
```
SSIM = (2μₓμᵧ + C₁)(2σₓᵧ + C₂) / (μₓ² + μᵧ² + C₁)(σₓ² + σᵧ² + C₂)
```

### YCbCr from RGB (ITU-R BT.601)
```
Y  =  0.299·R + 0.587·G + 0.114·B − 128
Cb = −0.168736·R − 0.331264·G + 0.5·B
Cr =  0.5·R − 0.418688·G − 0.081312·B
```

---

## Quick Reference — File Roles

| File | Package | Role |
|------|---------|------|
| [cmd/server/main.go](../cmd/server/main.go) | `main` | HTTP routes, server startup |
| [internal/core/dct/transform.go](../internal/core/dct/transform.go) | `dct` | ForwardDCT2D, InverseDCT2D, precomputed cosines |
| [internal/core/dct/quantization.go](../internal/core/dct/quantization.go) | `dct` | ISO tables, quality scaling, quantize/dequantize |
| [internal/core/dct/encoder.go](../internal/core/dct/encoder.go) | `dct` | Full encode pipeline, zigzag, DC delta, packChannels |
| [internal/core/dct/decoder.go](../internal/core/dct/decoder.go) | `dct` | Full decode pipeline, bilinear upsample, unpackChannels |
| [internal/core/huffman/encoder.go](../internal/core/huffman/encoder.go) | `huffman` | Tree building, encode, decode, bit packing |
| [internal/core/metrics/psnr.go](../internal/core/metrics/psnr.go) | `metrics` | MSE, CalculatePSNR |
| [internal/core/metrics/ssim.go](../internal/core/metrics/ssim.go) | `metrics` | CalculateSSIM, windowStats |
| [internal/handlers/handlers.go](../internal/handlers/handlers.go) | `handlers` | Upload, Compress, Benchmark, Download, Export handlers |
| [internal/handlers/health.go](../internal/handlers/health.go) | `handlers` | HealthHandler |
| [internal/models/models.go](../internal/models/models.go) | `models` | CompressionResult, BenchmarkResult, ImageMetadata |
| [internal/services/compression_service.go](../internal/services/compression_service.go) | `services` | CompressionService, CompressGoJPEG |
| [internal/services/benchmark_service.go](../internal/services/benchmark_service.go) | `services` | BenchmarkService.RunAll |
| [internal/python/bridge.go](../internal/python/bridge.go) | `python` | Bridge, runScript, saveTempPNG |
| [internal/python/scripts/compress_pil.py](../internal/python/scripts/compress_pil.py) | Python | PIL JPEG benchmark |
| [internal/python/scripts/compress_opencv.py](../internal/python/scripts/compress_opencv.py) | Python | OpenCV JPEG benchmark |
| [internal/io/image_loader.go](../internal/io/image_loader.go) | `io` | LoadImage, format validation |
| [internal/io/image_writer.go](../internal/io/image_writer.go) | `io` | WriteImage, EncodeImageToBytes |
| [web/index.html](../web/index.html) | HTML | Single-page app shell |
| [web/app.js](../web/app.js) | JS | Upload, compress, benchmark, heatmap, export UI |
| [web/styles.css](../web/styles.css) | CSS | Dark/light theme, glassmorphism |
| [Dockerfile](../Dockerfile) | — | Multi-stage build: Go builder + Python runtime |
| [Makefile](../Makefile) | — | Build, test, run, docker targets |
| [go.mod](../go.mod) | — | Module name, Go version, one dependency |

---

## Key Design Decisions & Their Rationale

| Decision | Alternative Considered | Why This Was Chosen |
|----------|----------------------|---------------------|
| Separable DCT (2 passes) | Butterfly (AAN) | Simpler to implement correctly; 2.4× speedup sufficient |
| Per-image adaptive Huffman | Static JPEG Huffman tables | Adapts to image-specific statistics |
| EOB marker for AC | Run-length (zero, count) pairs | Tested RLE — it made things worse |
| DC step × 0.80 | No DC tuning | +0.08 dB PSNR for +0.4 KB; favorable trade |
| Bilinear chroma upsample | Nearest-neighbor | +0.15–0.3 dB PSNR for free (no size change) |
| 4:2:0 default subsampling | 4:4:4 | Industry standard; 50% data reduction before DCT |
| Go (not C++) | C++ | Goroutines, stdlib, escape analysis; 40 ms for 1080p |
| Python via subprocess | CGO binding | Simpler, isolated, testable independently |

---

## Benchmark Results Reference

**Quality = 75, Image = 1920×1080 (natural photo)**

| Codec | File Size | vs Raw | PSNR | SSIM | Encode |
|-------|-----------|--------|------|------|--------|
| DCTPress (this project) | 83.4 KB | 48.3× | 33.77 dB | 0.9924 | 179 ms* |
| Go stdlib JPEG | 86.9 KB | 46.4× | 33.84 dB | 0.9934 | 29 ms |
| Python PIL | 76.8 KB | — | 33.70 dB | 0.9822 | 7 ms |
| Python OpenCV | 86.7 KB | — | 33.70 dB | 0.9822 | 2 ms |

*179 ms = encode + decode + PSNR + SSIM. Encode alone: ~40 ms.

---

## Glossary

| Term | Definition |
|------|-----------|
| **AC coefficient** | Any DCT coefficient except DC; represents spatial variation |
| **Alpha factor** | Normalization: 1/√2 for k=0, 1 for k≥1 |
| **Bilinear interpolation** | Weighted average of 4 surrounding pixels for upsampling |
| **Cb, Cr** | Chroma difference channels (blue-shift, red-shift) |
| **Chroma subsampling** | Reducing color plane resolution (4:2:0, 4:2:2, 4:4:4) |
| **DC coefficient** | F(0,0) — represents the mean value of an 8×8 block |
| **DC delta coding** | Storing differences between consecutive blocks' DC values |
| **DCT** | Discrete Cosine Transform — converts pixels to frequencies |
| **Dequantization** | Multiply quantized integers by step size to get approximate DCT values |
| **Energy compaction** | DCT concentrates most signal energy in few low-frequency coefficients |
| **EOB marker** | End-of-Block sentinel (-32768) after last non-zero AC coefficient |
| **Huffman coding** | Prefix-free entropy code where frequent symbols get short bit strings |
| **Level shift** | Subtracting 128 from Y before DCT to center around zero |
| **MSE** | Mean Squared Error — average squared pixel difference |
| **Prefix-free code** | No valid codeword is a prefix of another (enables unambiguous decoding) |
| **PSNR** | Peak Signal-to-Noise Ratio — quality metric in decibels |
| **Quantization** | Divide DCT coefficients by step sizes and round to integers (the lossy step) |
| **Separable DCT** | 2D DCT as two sequential 1D passes (rows then columns) |
| **SSIM** | Structural Similarity Index — perceptual quality metric |
| **Worker pool** | Fixed set of goroutines pulling work from a shared channel |
| **YCbCr** | Color space separating luminance (Y) from chroma (Cb, Cr) |
| **Zigzag ordering** | Reading 8×8 block diagonally, low-frequency first |
