# DCTPress — Project Notes

## What I'm Building

A from-scratch DCT-based image compression engine in Go. The idea is to implement the core of JPEG — color space conversion, block DCT, quantization, Huffman coding — without using libjpeg or any codec library, then benchmark it against Go stdlib JPEG and Python PIL/OpenCV to see how close a hand-rolled implementation can get.

The secondary goal is a clean web UI where you can upload an image, compress it, see the difference heatmap, and run the benchmark side by side.

---

## Why This Project

I wanted to actually understand what happens inside a JPEG encoder rather than just calling `jpeg.Encode`. The DCT is one of those algorithms that shows up everywhere (JPEG, MP3, H.264) but most people treat it as a black box. Building it from the math spec forces you to understand every decision: why 8×8 blocks, why zigzag ordering, why the human eye tolerates chroma loss more than luma loss.

It also makes for a good portfolio piece because it's measurable — you can put actual numbers on it (PSNR, SSIM, compression ratio, encode time) and compare against industry baselines.

---

## Architecture Decisions

**Go for the core.** The hot path (DCT loops) runs at near-native speed, goroutines make the parallel block processing straightforward, and the stdlib HTTP server + JSON handling means zero external dependencies for the server.

**Separable 2D DCT instead of a butterfly.** Two sequential 1D passes (row-wise then column-wise) is mathematically equivalent to the direct 2D formula but drops complexity from O(n⁴) to O(n³) per block. A full Arai-Agui-Nakajima butterfly would be faster but requires careful fixed-point arithmetic. For a first implementation, separable + precomputed cosines is the right trade-off — about 2.4× faster than naive, easy to unit-test.

**Per-image adaptive Huffman instead of static tables.** Standard JPEG uses fixed tables tuned for average image statistics. Building a fresh tree per image adapts to the actual coefficient distribution. For images with unusual statistics (screenshots, synthetic graphics) this gives meaningfully better compression. The overhead is storing the tree in the file header, which only dominates for very small images.

**DC coefficient tuning.** After quality scaling, the DC step size gets an additional 20% reduction. The DC coefficient represents the mean luminance of the entire block — coarse quantization here makes the whole block look the wrong brightness. The cost is about 0.4 KB on a typical image; the gain is about 0.08 dB PSNR.

**Bilinear chroma upsampling on decode.** Go stdlib JPEG uses nearest-neighbor. Bilinear with JPEG cosited siting gives ~0.15–0.3 dB better PSNR at zero cost in compressed file size.

**EOB truncation instead of run-length encoding.** I tested explicit (zeroRun, value) pairs and it made things worse — file size went from ~97 KB to ~134 KB on a test image. The adaptive Huffman already assigns a 1-bit code to zero (the most frequent symbol), so explicit run-length pairs just add overhead. The EOB marker approach is simpler and more effective.

---

## Pipeline

```
RGB → YCbCr (BT.601, level-shift Y by -128)
    → chroma subsample (4:2:0 default, averaging not decimation)
    → split into 8×8 blocks (edge-replicate padding)
    → forward DCT (separable, precomputed cosines)
    → quantize (ISO Annex K tables, quality-scaled, DC step -20%)
    → zigzag flatten
    → DC delta encode (serial, each block depends on previous)
    → EOB truncate (drop trailing zeros, mark end with -32768)
    → adaptive Huffman encode (per-channel, per-image tree)
    → pack into .dct binary (27-byte header + 3 Huffman blobs)
```

Decode is the exact inverse. The parallel worker pool runs DCT + quantize + zigzag across all blocks simultaneously (one goroutine per CPU); DC delta encoding is serial afterward since it's order-dependent.

---

## File Format (.dct)

```
Offset  Size  Field
0       4     Magic "DCT\x02"
4       1     Version (2)
5       4     Width (uint32 LE)
9       4     Height (uint32 LE)
13      1     Quality
14      1     Chroma subsampling (0=444, 1=422, 2=420)
15      4     Y blob length
19      4     Cb blob length
23      4     Cr blob length
27      —     Y Huffman blob
—       —     Cb Huffman blob
—       —     Cr Huffman blob
```

Each Huffman blob is self-contained: it carries the tree (pre-order serialized) and the packed bitstream. The decoder needs no side channel.

---

## Benchmark Results (quality 75, 1920×1080)

| Codec | Size | Ratio | PSNR | SSIM | Encode |
|---|---|---|---|---|---|
| DCTPress | 83.4 KB | 48.3× | 33.77 dB | 0.9924 | 179 ms |
| Go stdlib JPEG | 86.9 KB | 46.4× | 33.84 dB | 0.9934 | 29 ms |
| Python PIL | 76.8 KB | — | 33.70 dB | 0.9822 | 7 ms |
| Python OpenCV | 86.7 KB | — | 33.70 dB | 0.9822 | 2 ms |

The 179 ms figure is the full cycle: encode + decode + PSNR + SSIM. Encode alone is ~40 ms.

The Python ratios aren't directly comparable — they measure against the uploaded file size (which may already be JPEG-compressed), not against raw RGB.

---

## What Didn't Work

**Aggressive deadzone quantization.** Setting a deadzone of 0.64× the step size for high-frequency coefficients (u+v > 8) increased compression by ~2% but dropped PSNR by 0.26 dB. Not worth it.

**Run-length AC encoding.** Described above. The adaptive Huffman already handles sparse AC sequences efficiently.

**Naive DCT.** The first version used the direct O(n⁴) formula. Correct but ~800 ms for a 1080p image. Separable passes with precomputed cosines brought it to ~40 ms.

---

## Implementation Journey

| Phase | What changed | Encode time | File size |
|---|---|---|---|
| 1 | Naive 2D DCT, raw int32 output | ~800 ms | very large |
| 2 | Separable DCT + precomputed cosines | ~40 ms | very large |
| 3 | Huffman entropy coding | ~40 ms | −60% |
| 4 | Bilinear chroma upsampling (decode) | ~40 ms | no change, +0.15 dB |
| 5 | DC coefficient 20% tighter step | ~40 ms | +0.4 KB, +0.08 dB |
| 6 | Parallel block processing | ~179 ms* | no change |

*179 ms is encode + decode + metrics. Encode alone stays ~40 ms.

---

## Things to Add Later

- Progressive encoding (low-frequency coefficients first, like progressive JPEG)
- Arithmetic entropy coding — should give 5–10% better compression than Huffman
- SIMD for the DCT butterfly
- Tiling for 4K+ images to reduce peak memory
- pprof endpoints behind a build tag
- WebP export (needs a Go WebP encoder, nothing in stdlib)
