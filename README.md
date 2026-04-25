# DCTPress — Image Compression Engine

<div align="center">

**A from-scratch DCT-based image codec with adaptive Huffman entropy coding, built in Go.**

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Docker Ready](https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker)](Dockerfile)
[![Python Bridge](https://img.shields.io/badge/Python-3.11-3776AB?style=flat-square&logo=python)](internal/python/)

</div>

---

## What This Is

DCTPress is a complete image compression pipeline written from first principles — no libjpeg, no codec bindings, no shortcuts. It implements the core ideas behind JPEG (block DCT, perceptual quantization, entropy coding) in clean Go, then exposes the codec through a REST API and a live benchmarking web UI that compares it against Go stdlib JPEG, Python PIL, and Python OpenCV side-by-side.

The goal was to understand every decision that goes into a real image codec: why 8×8 blocks, why zigzag ordering, why DC delta coding, why the human visual system tolerates more chroma loss than luma loss. Every one of those decisions is implemented and measurable in this codebase.

---

## Quick Start

```bash
# Prerequisites: Go 1.25+, Python 3.11+

git clone https://github.com/shashankc2507/imagecompressor-dct
cd imagecompressor-dct

make setup    # go mod tidy + pip install Python deps
make run      # http://localhost:8081

make restart  # rebuild and restart a running server
make test     # full test suite with coverage
```

With Docker:

```bash
docker compose up   # http://localhost:8080
docker compose down
```

---

## Benchmark Results (quality = 75, 1920×1080)

| Codec | File Size | Ratio | PSNR | SSIM | Encode |
|---|---|---|---|---|---|
| **DCTPress** | **83.4 KB** | **48.3×** | 33.77 dB | 0.9924 | 179 ms |
| Go stdlib JPEG | 86.9 KB | 46.4× | 33.84 dB | 0.9934 | 29 ms |
| Python PIL | 76.8 KB | — | 33.70 dB | 0.9822 | 7 ms |
| Python OpenCV | 86.7 KB | — | 33.70 dB | 0.9822 | 2 ms |

DCTPress produces a **4% smaller file than Go stdlib JPEG** at the same quality setting, with only 0.07 dB PSNR difference. The trade-off is ~6× slower encoding (179 ms vs 29 ms) — acceptable for archival use, not for real-time pipelines.

The Python PIL/OpenCV ratios are not directly comparable because they measure compression against the uploaded file size (which may already be JPEG-compressed), not against raw RGB.

---

## Architecture

```
Browser  ──multipart──▶  HTTP API (:8081)
                              │
                    ┌─────────┼──────────┐
                    ▼         ▼          ▼
               /compress  /benchmark  /export
                    │         │
                    ▼         ▼
              DCT Encoder   BenchmarkService
              DCT Decoder      │
              PSNR / SSIM      ├── Go stdlib JPEG
                               ├── Python PIL (subprocess)
                               └── Python OpenCV (subprocess)
```

### Project Layout

```
imagecompressor-dct/
├── cmd/server/main.go              # HTTP server, route registration
├── internal/
│   ├── core/
│   │   ├── dct/
│   │   │   ├── transform.go        # Separable 2D DCT / IDCT, precomputed cosines
│   │   │   ├── quantization.go     # ISO Annex K tables, quality scaling, DC tuning
│   │   │   ├── encoder.go          # Full encode pipeline, parallel block processing
│   │   │   └── decoder.go          # Full decode pipeline, bilinear chroma upsample
│   │   ├── huffman/
│   │   │   └── encoder.go          # Min-heap tree builder, bitstream packer/unpacker
│   │   └── metrics/
│   │       ├── psnr.go             # MSE → PSNR in dB
│   │       └── ssim.go             # Sliding-window SSIM on luminance channel
│   ├── handlers/handlers.go        # Upload, Compress, Benchmark, Download, Export
│   ├── models/models.go            # Shared domain types
│   ├── services/
│   │   ├── compression_service.go  # Orchestrates encode + decode + metrics
│   │   └── benchmark_service.go    # Runs all four codecs, collects results
│   └── python/
│       ├── bridge.go               # Subprocess runner, stderr capture, JSON parse
│       └── scripts/
│           ├── compress_pil.py     # PIL JPEG benchmark
│           └── compress_opencv.py  # OpenCV JPEG benchmark
├── web/
│   ├── index.html                  # Single-page app
│   ├── app.js                      # Upload, compress, benchmark, heatmap, export
│   └── styles.css                  # Dark/light theme, animations, glassmorphism
├── test/
│   ├── unit/dct/                   # DCT transform + round-trip tests
│   ├── unit/huffman/               # Huffman encode/decode tests
│   ├── unit/metrics/               # PSNR/SSIM tests
│   └── integration/api_test.go     # End-to-end HTTP tests
├── Dockerfile                      # Multi-stage: Go builder + python:3.11-slim
├── docker-compose.yml
└── Makefile
```

---

## The Compression Pipeline — Step by Step

### 1. Color Space Conversion (RGB → YCbCr)

Every pixel is converted from RGB to YCbCr using the ITU-R BT.601 coefficients:

```
Y  =  0.299·R + 0.587·G + 0.114·B  − 128   (level-shifted to centre at 0)
Cb = −0.168736·R − 0.331264·G + 0.5·B
Cr =  0.5·R − 0.418688·G − 0.081312·B
```

The level shift (−128 on Y) centres the values around zero before the DCT. This matters because the DCT assumes a zero-mean signal — without the shift, the DC coefficient would be enormous and the quantization step would waste precision on it.

Y carries luminance (brightness). Cb and Cr carry colour difference signals. The human visual system is roughly 4× less sensitive to spatial detail in colour than in brightness, which is why chroma can be subsampled without visible quality loss.

### 2. Chroma Subsampling

Before encoding, the Cb and Cr planes are spatially downsampled:

| Mode | Cb/Cr dimensions | Chroma samples saved |
|---|---|---|
| 4:4:4 | full resolution | 0% |
| 4:2:2 | half width, full height | 50% |
| 4:2:0 | half width, half height | 75% |

4:2:0 is the default and the same mode used by standard JPEG. Downsampling is done by averaging 2×2 pixel regions (for 4:2:0) or adjacent horizontal pairs (for 4:2:2), not by simple decimation — this avoids aliasing at colour edges.

### 3. Block Splitting and Padding

Each channel plane is divided into non-overlapping 8×8 blocks. If the image dimensions are not multiples of 8, the edges are padded by replicating the last row/column. This is the same approach used by JPEG and avoids introducing artificial edges that would create high-frequency DCT coefficients.

### 4. Forward DCT (Separable 2D)

The core of the codec. Each 8×8 block of pixel values is transformed into 8×8 frequency coefficients using the 2D Discrete Cosine Transform.

**Why DCT?** The DCT has excellent energy compaction for natural images — most of the signal energy ends up in a small number of low-frequency coefficients. The high-frequency coefficients (fine detail, sharp edges) are small and can be coarsely quantized or discarded entirely without visible quality loss.

**Separable implementation:** The 2D DCT is computed as two sequential 1D passes — once across rows, once down columns. This is mathematically equivalent to the direct 2D formula but reduces complexity from O(n⁴) to O(n³) per block.

```go
// Forward 1D DCT-II (length 8)
// Output[k] = 0.5 · α(k) · Σ_n input[n] · cos((2n+1)kπ/16)
func forwardDCT1D(x [8]float64) [8]float64

// 2D DCT: row pass → column pass
func ForwardDCT2D(block [8][8]float64) [8][8]float64
```

The cosine values `cos((2n+1)kπ/16)` for all combinations of n and k are precomputed at package init into a `[8][8]float64` lookup table. This gives a ~2.4× speedup over computing `math.Cos` on every coefficient.

**Parallel block processing:** The DCT, quantization, and zigzag steps are embarrassingly parallel — each block is independent. The encoder dispatches all blocks to a worker pool sized to `runtime.NumCPU()` via a channel, then collects results in order. DC delta encoding is done serially afterward since each block's DC value depends on the previous block's DC value.

### 5. Quantization

Each DCT coefficient `F(u,v)` is divided by a perceptual step size `Q(u,v)` and rounded to the nearest integer:

```
quantized(u,v) = round( F(u,v) / Q(u,v) )
```

The step sizes come from the ISO JPEG Annex K tables — two 8×8 matrices derived from human contrast sensitivity research. Low-frequency coefficients (top-left of the block) get small step sizes (high precision). High-frequency coefficients (bottom-right) get large step sizes (coarse precision or zero).

**Quality scaling:** The JPEG reference formula maps quality 1–100 to a scale multiplier:

```go
if quality < 50 {
    scale = 50.0 / float64(quality)   // aggressive: quality 1 → scale 50×
} else {
    scale = 2.0 - float64(quality)/50.0  // fine: quality 100 → scale 0 (lossless)
}
```

Each base table entry is multiplied by this scale and clamped to [1, 255].

**DC coefficient tuning:** The DC coefficient `F(0,0)` represents the mean luminance of the block. It has outsized perceptual impact — a coarsely quantized DC value makes the entire block look the wrong brightness. DCTPress applies an additional 20% reduction to the DC step size (`value × 0.80`) after quality scaling. This costs ~0.4 KB on a typical image but recovers ~0.08 dB PSNR.

### 6. Zigzag Ordering

The 8×8 quantized block is serialised into a 64-element sequence by traversing diagonals from the DC coefficient (top-left, lowest frequency) to the highest spatial frequency (bottom-right):

```
DC → (0,1) → (1,0) → (2,0) → (1,1) → (0,2) → (0,3) → ...
```

This ordering clusters the many near-zero high-frequency coefficients at the end of the sequence, which maximises the effectiveness of the EOB marker in the next step.

### 7. DC Delta Coding + EOB Truncation

Two techniques reduce the symbol count before entropy coding:

**DC delta coding:** Instead of storing each block's DC value directly, only the *difference* from the previous block's DC value is stored. Adjacent blocks in natural images tend to have similar mean luminance, so these deltas are small and cluster near zero — ideal for Huffman coding.

**EOB truncation:** After the last non-zero AC coefficient in the zigzag sequence, an End-of-Block marker (`−32768`, which cannot appear as a valid quantized coefficient) is written. All trailing zeros are implicit. For a typical image at quality 75, most blocks have only 5–15 non-zero AC coefficients out of 63, so this alone cuts the symbol count by 4–8×.

The per-block stream looks like:

```
[dc_delta]  [ac_1] [ac_2] ... [ac_k]  [EOB=-32768]
```

### 8. Adaptive Huffman Entropy Coding

The integer coefficient stream is compressed with a Huffman code built fresh for each image and each channel (Y, Cb, Cr independently).

**Why per-image?** A static Huffman table (like standard JPEG uses) is tuned for average image statistics. A per-image table adapts to the actual coefficient distribution of this specific image — a mostly-flat image has very different statistics from a high-detail photograph.

**Tree construction:** Symbol frequencies are counted in a single pass. A min-heap priority queue builds the optimal prefix-free tree in O(n log n) time by repeatedly merging the two lowest-frequency nodes.

**Wire format:** The tree is serialised in pre-order into the bitstream header so the decoder can reconstruct it without any side channel:

```
[symbolCount(4)]
[treeNodeCount(4)]
[tree nodes, pre-order: 'L'+symbol(2) for leaves, 'I' for internal]
[paddingBits(1)]
[packed bitstream...]
```

Single-symbol inputs (e.g. a completely flat channel) are handled as a special case: just `[1][symbol][count]`, avoiding the overhead of a degenerate tree.

**Bit packing:** The variable-length bit strings are packed MSB-first into bytes. The number of padding bits in the final byte is stored so the decoder knows where the valid bits end.

### 9. Binary File Format (.dct)

The complete compressed file is a single byte slice with a 27-byte header:

```
Offset  Size  Field
0       4     Magic: "DCT\x02"
4       1     Version: 2
5       4     Width (uint32 LE)
9       4     Height (uint32 LE)
13      1     Quality (1–100)
14      1     Chroma subsampling mode (0=444, 1=422, 2=420)
15      4     Y channel Huffman blob length
19      4     Cb channel Huffman blob length
23      4     Cr channel Huffman blob length
27      —     Y Huffman blob
—       —     Cb Huffman blob
—       —     Cr Huffman blob
```

Each channel is Huffman-encoded independently, which lets the entropy coder adapt to the different statistical distributions of luma vs chroma coefficients.

---

## The Decode Pipeline

Decoding is the exact inverse of encoding:

1. Parse the 27-byte header to recover dimensions, quality, and subsampling mode
2. Huffman-decode each channel blob back to the integer coefficient stream
3. For each block: undo DC delta → zigzag unflatten → dequantize → Inverse DCT 2D
4. Bilinear chroma upsample (if subsampled)
5. YCbCr → RGB conversion and clamp to [0, 255]

**Bilinear chroma upsampling:** On decode, the subsampled chroma planes are expanded back to full luma resolution using bilinear interpolation with JPEG cosited siting:

```go
// Maps target pixel centre to source coordinate
srcY := (float64(row) + 0.5) / scaleY - 0.5
srcX := (float64(col) + 0.5) / scaleX - 0.5

// Four-sample bilinear blend
result = tl*(1-fx)*(1-fy) + tr*fx*(1-fy) +
         bl*(1-fx)*fy     + br*fx*fy
```

The `+0.5 / scale − 0.5` formula aligns pixel centres rather than pixel corners (JPEG cosited siting). This gives ~0.15–0.3 dB better PSNR than nearest-neighbor at zero cost in compressed file size.

---

## Quality Metrics

After every compression, DCTPress decodes the result and measures two quality metrics against the original:

### PSNR (Peak Signal-to-Noise Ratio)

```
MSE  = mean( (original_pixel − compressed_pixel)² )  over all pixels and channels
PSNR = 10 · log₁₀( 255² / MSE )  dB
```

Higher is better. Returns +∞ for identical images.

| Range | Interpretation |
|---|---|
| < 30 dB | Noticeable degradation |
| 30–35 dB | Acceptable for most uses |
| 35–40 dB | Good quality |
| > 40 dB | Excellent, near-lossless |

### SSIM (Structural Similarity Index)

SSIM measures perceptual similarity by comparing local luminance, contrast, and structure in overlapping 8×8 windows across the image. It uses the Wang et al. 2004 formula with standard stability constants (k1=0.01, k2=0.03):

```
SSIM(x,y) = (2μₓμᵧ + C₁)(2σₓᵧ + C₂)
            ─────────────────────────────
            (μₓ² + μᵧ² + C₁)(σₓ² + σᵧ² + C₂)
```

Windows step by 4 pixels (half the window size) for dense coverage. The final score is the mean over all windows. Range is [0, 1]; values above 0.95 are excellent.

SSIM is computed on the luminance channel only, which matches how the human visual system perceives image quality.

---

## Python Benchmark Bridge

The benchmark runner compares DCTPress against Python PIL and OpenCV by spawning them as subprocesses:

```go
cmd := exec.CommandContext(ctx, "python3", scriptPath,
    "--input", imagePath, "--quality", quality, "--output-json")
```

The image is written to a temp PNG file, passed to the script, and the script returns a JSON result on stdout. Stderr is captured separately so Python tracebacks appear in the Go server logs rather than being silently swallowed.

Both Python scripts use `scikit-image` for PSNR/SSIM calculation and the deprecated `multichannel` kwarg has been removed (replaced with `channel_axis=2`) to avoid warnings that could corrupt JSON parsing.

If Python is unavailable or a script fails, those results are simply omitted from the benchmark — the Go codecs always run.

---

## Web UI

The single-page app at `http://localhost:8081` provides:

- **Drag-and-drop upload** with instant preview
- **Quality slider** (1–100) and chroma subsampling selector
- **Three-panel comparison**: original / difference heatmap / decompressed
- **Difference heatmap**: per-pixel error amplified and colour-coded green→yellow→red, with adjustable amplification (1–20×)
- **Metric cards**: file size, compression ratio, PSNR, SSIM, encode time — each with a progress bar and quality classification
- **Image zoom**: click any panel image to open a fullscreen modal
- **Benchmark table**: all four codecs with size bars, badges, hero stats strip, and insights panel
- **Export**: download as DCT binary (exact compressed size), JPEG (re-encoded at original quality), or PNG
- **Dark/light theme** toggle
- **Keyboard shortcuts**: `Ctrl+U` upload, `Ctrl+Enter` compress, `Ctrl+S` download DCT

---

## API Reference

Base URL: `http://localhost:8081`

### `POST /api/upload`

Upload an image. Returns a session ID used by all subsequent endpoints.

```bash
curl -X POST http://localhost:8081/api/upload -F "image=@photo.jpg"
```

```json
{ "id": "img_1", "format": "jpeg", "width": 1920, "height": 1080,
  "size_bytes": 2097152, "color_mode": "RGB" }
```

### `POST /api/compress`

Compress an uploaded image with DCTPress.

```bash
curl -X POST http://localhost:8081/api/compress \
  -H "Content-Type: application/json" \
  -d '{"image_id":"img_1","quality":75,"chroma_subsampling":"4:2:0"}'
```

| Field | Type | Default | Description |
|---|---|---|---|
| `image_id` | string | required | ID from `/api/upload` |
| `quality` | int | — | 1–100 |
| `chroma_subsampling` | string | `"4:2:0"` | `"4:2:0"`, `"4:2:2"`, `"4:4:4"` |

```json
{
  "export_id": "dl_1",
  "compressed_image_url": "/api/download/dl_1",
  "preview_url": "/api/download/dl_2",
  "compressed_size": 85401,
  "compression_ratio": 48.29,
  "file_original_size": 2097152,
  "file_ratio": 24.56,
  "psnr": 33.77,
  "ssim": 0.9924,
  "encoding_time_ms": 179
}
```

### `POST /api/benchmark`

Run all four codecs on an uploaded image and return comparison data.

```bash
curl -X POST http://localhost:8081/api/benchmark \
  -H "Content-Type: application/json" \
  -d '{"image_id":"img_1","quality":75}'
```

### `GET /api/export?id=<export_id>&format=<jpeg|png|dct>`

Download the compressed image in the requested format.

- `dct` — raw `.dct` binary (exact compressed size, no re-encoding overhead)
- `jpeg` — decoded image re-encoded as JPEG at the original quality setting
- `png` — decoded image encoded as lossless PNG

```bash
curl "http://localhost:8081/api/export?id=dl_1&format=dct" -o compressed.dct
curl "http://localhost:8081/api/export?id=dl_1&format=jpeg" -o compressed.jpg
```

### `GET /api/download/<id>`

Serve a raw stored blob (used internally for preview images).

### `GET /api/formats`

```json
{ "input_formats": ["png","jpeg","webp","bmp","tiff"],
  "output_formats": ["dct","jpeg","png"] }
```

### `GET /health`

Liveness probe. Returns `200 OK`.

---

## Makefile Reference

| Target | Description |
|---|---|
| `make run` | `go run` the server on port 8081 |
| `make restart` | Kill port 8081, rebuild binary, start server |
| `make build` | Compile to `bin/server` |
| `make build-prod` | Compile with `-ldflags="-s -w"` (stripped binary) |
| `make test` | Full test suite with coverage |
| `make test-race` | Tests with Go race detector |
| `make benchmark` | Go microbenchmarks in `test/benchmarks/` |
| `make coverage` | Coverage report, opens browser |
| `make lint` | `golangci-lint run` |
| `make format` | `gofmt -w -s .` |
| `make setup` | `go mod tidy` + pip install Python deps |
| `make clean` | Remove `bin/` and `tmp/` |
| `make docker-up` | `docker compose up -d` |
| `make docker-down` | `docker compose down` |
| `make docker-logs` | Tail container logs |

---

## Design Decisions

### Why 8×8 blocks?

8×8 is the JPEG standard block size. It maps well to CPU cache lines, the 8-point DCT has efficient butterfly implementations, and the ISO Annex K quantization tables are specifically calibrated for 8×8 blocks. Larger blocks (16×16, 32×32) give better energy compaction but produce more visible ringing artifacts at block boundaries when heavily quantized.

### Why separable DCT instead of a butterfly algorithm?

The separable two-pass approach is simpler to implement correctly and verify. A full butterfly (Arai-Agui-Nakajima or similar) would be faster but requires careful fixed-point arithmetic to avoid numerical drift. For a from-scratch implementation where correctness is the priority, separable + precomputed cosines gives a good balance: ~2.4× faster than naive, easy to unit-test against the direct formula.

### Why per-image adaptive Huffman instead of static tables?

Standard JPEG uses fixed Huffman tables derived from average image statistics. A per-image tree adapts to the actual coefficient distribution of each image. For images with unusual statistics (synthetic graphics, screenshots, medical images) this can give meaningfully better compression. The cost is that the tree must be stored in the file header — for small images this overhead dominates, but for anything above ~100×100 pixels the adaptive tree wins.

### Why not arithmetic coding?

Arithmetic coding (used in JPEG 2000, HEVC) achieves ~5–10% better entropy coding than Huffman. It was not used here because implementing a correct range coder with carry propagation and range normalization is significantly more complex, and the goal was to understand the codec pipeline rather than squeeze out the last few percent of compression.

### Why not run-length AC encoding?

Run-length encoding of AC coefficients (storing `(zeroRun, value)` pairs instead of individual values) was tested and made things worse: file size increased from ~97 KB to ~134 KB on a test image. The reason: the adaptive Huffman had already assigned a 1-bit code to zero (the most frequent symbol). Explicit run-length pairs eliminated that advantage and added extra symbols for non-sparse blocks. The EOB marker approach is simpler and more effective for this entropy coder.

### Why Go instead of C++?

C++ would be faster, but Go's escape analysis, goroutine model, and standard library made it the right trade-off for a single-author project. The hot path (DCT loops) runs at near-native speed. The concurrency model (worker pool via channels) is straightforward to reason about. The HTTP server and JSON handling are zero-dependency. The result is ~40 ms encode time for a 1080p image — fast enough for interactive use.

---

## What Didn't Work

**Aggressive deadzone quantization** — setting a deadzone of 0.64× the step size for high-frequency coefficients (u+v > 8) increased compression by ~2% but dropped PSNR by 0.26 dB. Not worth it.

**Run-length AC encoding** — described above. The adaptive Huffman already handles sparse AC sequences efficiently via short codes for zero.

**Naive DCT** — the first implementation used the direct O(n⁴) formula. Correct but slow: ~800 ms for a 1080p image. Switching to separable passes with precomputed cosines brought this to ~40 ms. Adding the parallel worker pool brought it to the current ~179 ms for a full encode+decode+metrics cycle.

---

## Implementation Journey

| Phase | Change | Encode time | File size |
|---|---|---|---|
| 1 | Naive 2D DCT, raw int32 output | ~800 ms | very large |
| 2 | Separable DCT + precomputed cosines | ~40 ms | very large |
| 3 | Huffman entropy coding | ~40 ms | −60% |
| 4 | Bilinear chroma upsampling (decode) | ~40 ms | no change, +0.15 dB PSNR |
| 5 | DC coefficient 20% tighter step | ~40 ms | +0.4 KB, +0.08 dB PSNR |
| 6 | Parallel block processing (goroutines) | ~179 ms* | no change |

*The current 179 ms includes encode + decode + PSNR + SSIM calculation. Encode alone is ~40 ms.

---

## Future Roadmap

- [ ] Progressive encoding — write low-frequency coefficients first for progressive JPEG-style loading
- [ ] Arithmetic entropy coding — replace adaptive Huffman with a range coder for 5–10% better compression
- [ ] SIMD acceleration — platform intrinsics for the DCT butterfly operations
- [ ] Tiling for large images — process tiles in parallel goroutines to reduce peak memory on 4K+ inputs
- [ ] pprof integration — expose `net/http/pprof` endpoints behind a build tag
- [ ] WebP output — encode decoded `.dct` to WebP (requires a Go WebP encoder)

---

## License

MIT — see [LICENSE](LICENSE).

---

<div align="center">
Built from scratch by <strong>Shashank S</strong>
</div>
