# Phase 00 — Overview & Mental Model

> *Before writing a single line of code, you need a map of the territory. This phase gives you that map — what DCTPress is, why it exists, and how every piece connects to every other piece.*

---

## What Is DCTPress?

DCTPress is a **from-scratch image compression engine** implemented in Go. It replicates the core ideas behind the JPEG standard — without using any JPEG library — and wraps everything in a REST API with a live benchmarking web UI.

The name comes from **DCT** (Discrete Cosine Transform), the mathematical heart of JPEG compression, and **Press**, as in "compress."

### The One-Sentence Summary

> DCTPress takes a raw image, shrinks it to a fraction of its original size by selectively throwing away visual information the human eye cannot detect, and can reconstruct a visually indistinguishable version from that shrunken form.

---

## Why Build This From Scratch?

### The "just use libjpeg" argument

Every real-world application uses battle-tested libraries: `libjpeg`, `libpng`, `imagemagick`. These are fast, correct, and maintained by thousands of engineers. So why re-implement?

**Because understanding beats usage.** When you call `jpeg.Encode(img, quality=75)`, you get a number. You don't know:

- Why 8×8 blocks and not 16×16?
- Why is the DC coefficient special?
- What is actually being "lost" in lossy compression?
- Why does PSNR matter and what does 33 dB mean?
- Why does chroma subsampling work without visible quality loss?

Building from scratch forces you to answer every one of these questions with working code.

### The Real-World Analogy

Think of JPEG compression like **describing a painting over the phone to someone who will recreate it**:

- You don't describe every pixel — you describe patterns: "the sky is a smooth gradient from dark blue at the top to light blue at the horizon"
- You spend more words on important details (faces, sharp edges) and fewer words on textures the eye glosses over (a uniform brick wall)
- Some information is lost (the exact shade of blue at pixel (342, 219)) but the overall impression is preserved
- DCT is the "language" you use to describe patterns efficiently; Huffman coding is the shorthand that compresses that description further

---

## The Full Pipeline — Bird's Eye View

```
┌──────────────────────────────────────────────────────────────────┐
│                         ENCODE PATH                              │
│                                                                  │
│  Raw Image (RGB pixels)                                          │
│       │                                                          │
│       ▼                                                          │
│  1. Color Space Conversion   RGB → YCbCr                         │
│       │   (separate brightness from color)                       │
│       ▼                                                          │
│  2. Chroma Subsampling       Cb, Cr planes → half resolution     │
│       │   (human eye cares less about color detail)              │
│       ▼                                                          │
│  3. Block Splitting          each plane → 8×8 tiles              │
│       │   (process small independent pieces)                     │
│       ▼                                                          │
│  4. Forward DCT              pixel values → frequency spectrum   │
│       │   (find what frequencies are present)                    │
│       ▼                                                          │
│  5. Quantization             divide by step sizes, round         │
│       │   (throw away imperceptible high-freq detail)            │
│       ▼                                                          │
│  6. Zigzag Ordering          2D block → 1D sequence              │
│       │   (cluster zeros at the end)                             │
│       ▼                                                          │
│  7. DC Delta Coding + EOB    encode differences, drop trailing 0s│
│       │   (exploit inter-block correlation and sparsity)         │
│       ▼                                                          │
│  8. Huffman Entropy Coding   integers → compact bit strings      │
│       │   (frequent symbols get short codes)                     │
│       ▼                                                          │
│  9. Binary Packing           header + Y/Cb/Cr blobs → .dct file  │
└──────────────────────────────────────────────────────────────────┘

                         ▲ DECODE PATH (exact reverse) ▼
```

Each step is reversible (except quantization — that's where the "lossy" happens). The decoder runs each step in reverse order to reconstruct the image.

---

## File Map — Every File and Its Role

```
imagecompressor-dct/
│
├── cmd/server/main.go              ← Program entry point. Registers HTTP routes, starts server.
│
├── internal/                       ← All internal packages (not importable by external code)
│   │
│   ├── core/                       ← Pure algorithm implementations (no HTTP, no I/O)
│   │   ├── dct/
│   │   │   ├── transform.go        ← The DCT math: ForwardDCT2D, InverseDCT2D
│   │   │   ├── quantization.go     ← Perceptual step tables, quality scaling
│   │   │   ├── encoder.go          ← Full encode pipeline (RGB→packed .dct bytes)
│   │   │   └── decoder.go          ← Full decode pipeline (.dct bytes → RGB image)
│   │   ├── huffman/
│   │   │   └── encoder.go          ← Huffman tree, bitstream encoder/decoder
│   │   └── metrics/
│   │       ├── psnr.go             ← MSE and PSNR calculation
│   │       └── ssim.go             ← SSIM calculation (perceptual quality)
│   │
│   ├── handlers/                   ← HTTP layer (parse request, call service, write response)
│   │   ├── handlers.go             ← Upload, Compress, Benchmark, Download, Export, Formats
│   │   └── health.go               ← GET /health liveness probe
│   │
│   ├── models/models.go            ← Shared data types (CompressionResult, BenchmarkResult, ...)
│   │
│   ├── services/                   ← Business logic (orchestrates core + python + metrics)
│   │   ├── compression_service.go  ← Encode + decode + PSNR + SSIM in one call
│   │   └── benchmark_service.go    ← Runs all four codecs, aggregates results
│   │
│   ├── io/                         ← File I/O utilities
│   │   ├── image_loader.go         ← Load image from disk or bytes (PNG, JPEG, WebP, BMP, TIFF)
│   │   └── image_writer.go         ← Write image to disk or bytes (JPEG, PNG)
│   │
│   └── python/                     ← Python interop layer
│       ├── bridge.go               ← subprocess runner, JSON parser
│       └── scripts/
│           ├── compress_pil.py     ← PIL JPEG benchmark script
│           └── compress_opencv.py  ← OpenCV JPEG benchmark script
│
├── web/                            ← Browser UI (single-page app)
│   ├── index.html                  ← HTML shell
│   ├── app.js                      ← Upload, compress, benchmark, heatmap, export
│   └── styles.css                  ← Dark/light theme, glassmorphism
│
├── test/
│   ├── unit/dct/                   ← DCT transform and encoder/decoder tests
│   ├── unit/huffman/               ← Huffman encode/decode tests
│   ├── unit/metrics/               ← PSNR/SSIM tests
│   └── integration/api_test.go     ← End-to-end HTTP tests
│
├── Dockerfile                      ← Multi-stage: Go builder + python:3.11-slim runtime
├── docker-compose.yml              ← Single-command deployment
├── Makefile                        ← Build, test, run, format, lint targets
└── go.mod                          ← Module name + Go version + dependencies
```

---

## Key Concepts Roadmap

Each phase of this documentation covers one major concept. Here is what is coming and why each matters:

| Phase | Concept | Why It Matters |
|-------|---------|----------------|
| 01 | Go language & module system | The language this is written in |
| 02 | Image fundamentals & color spaces | Data model before compression |
| 03 | The DCT algorithm | The mathematical core |
| 04 | Quantization & human perception | Where the compression actually happens |
| 05 | Full encode pipeline | How all pieces assemble |
| 06 | Full decode pipeline | The inverse journey |
| 07 | Huffman entropy coding | Compact bit representation |
| 08 | Quality metrics (PSNR, SSIM) | Measuring what was lost |
| 09 | HTTP API & service layer | How the world accesses the codec |
| 10 | Python bridge & benchmarking | Comparing against real-world codecs |
| 11 | Testing strategy | Proving correctness |
| 12 | Infrastructure (Docker, Makefile) | Deploying and operating |

---

## Performance Snapshot

At quality=75 on a 1920×1080 image:

| What | Numbers |
|------|---------|
| Raw RGB size | ~6.2 MB (1920 × 1080 × 3 bytes) |
| Compressed .dct size | ~83.4 KB |
| Compression ratio | **48.3×** |
| PSNR | 33.77 dB (acceptable, perceptually good) |
| SSIM | 0.9924 (excellent) |
| Encode time | ~179 ms (encode + decode + metrics) |

The 179 ms is the full pipeline including decode and metric calculation. Encode alone is ~40 ms.

---

## Design Philosophy

Every major decision in DCTPress follows one principle:

> **Understand before optimize.**

- Separable DCT instead of butterfly: simpler to verify, still 2.4× faster than naive
- Per-image Huffman instead of static tables: adapts to the actual data
- Bilinear chroma upsampling instead of nearest-neighbor: better PSNR for free
- Go instead of C++: goroutine model maps naturally to embarrassingly parallel block processing

The README contains a "What Didn't Work" section. That section is as valuable as the code — it documents dead ends so future developers don't repeat them.

---

*Next: [Phase 01 — Go Language & Module System](01_go_language_and_module_system.md)*
