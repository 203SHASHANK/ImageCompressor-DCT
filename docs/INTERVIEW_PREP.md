# Interview Prep — DCTPress Image Compression Engine

Everything you need to answer interview questions about this project at any depth: internship, new-grad, SDE-2, or staff level. Organised from "explain it to a non-engineer" up to "justify every bit of the wire format".  

### If You Are Using My Project For Interview Purpose DO Not Just Read This DOCS Pleaes Trace And Learn Each steps in Algrithm implementation andn then read this Out .

---

## Table of Contents

1. [Project Pitch (30 seconds / 2 minutes)](#1-project-pitch)
2. [Image Compression Theory](#2-image-compression-theory)
3. [DCT — The Core Algorithm](#3-dct--the-core-algorithm)
4. [Quantization & Perceptual Coding](#4-quantization--perceptual-coding)
5. [Entropy Coding — Huffman](#5-entropy-coding--huffman)
6. [Color Spaces & Chroma Subsampling](#6-color-spaces--chroma-subsampling)
7. [Quality Metrics — PSNR & SSIM](#7-quality-metrics--psnr--ssim)
8. [Data Structures & Algorithms](#8-data-structures--algorithms)
9. [Complexity Analysis](#9-complexity-analysis)
10. [Go Language Deep Dive](#10-go-language-deep-dive)
11. [System Design](#11-system-design)
12. [Performance & Optimization](#12-performance--optimization)
13. [Testing Strategy](#13-testing-strategy)
14. [API & HTTP Design](#14-api--http-design)
15. [Docker & Deployment](#15-docker--deployment)
16. [Python Bridge / IPC](#16-python-bridge--ipc)
17. [Design Trade-offs & Decisions](#17-design-trade-offs--decisions)
18. [Behavioural / STAR Questions](#18-behavioural--star-questions)
19. [Trick / Gotcha Questions](#19-trick--gotcha-questions)
20. [What Would You Do Differently?](#20-what-would-you-do-differently)

---

## 1. Project Pitch

### "Tell me about this project in 30 seconds."

> DCTPress is a from-scratch image codec I built in Go. It implements the same core algorithm that JPEG uses — Discrete Cosine Transform, quantization, Huffman entropy coding — but written entirely from first principles with no codec libraries. It achieves a 53× compression ratio on typical images and comes with a benchmarking server that compares it live against Go stdlib JPEG, Python PIL, and OpenCV.

### "Walk me through it in 2 minutes."

Start with the problem: a raw 1080p image is ~6 MB. We want ~38 KB. The trick is to exploit how human vision works.

Step 1 — **Color space**: Convert RGB to YCbCr. The Y channel (brightness) carries most visual detail. Cb and Cr (color difference) can be halved in resolution without visible quality loss — that's chroma subsampling.

Step 2 — **DCT**: Split each channel into 8×8 blocks and apply the Discrete Cosine Transform. This converts pixel values into frequency coefficients. The DC coefficient (position [0,0]) is the block's average brightness. Most of the perceptual energy concentrates in the top-left low-frequency corner.

Step 3 — **Quantization**: Divide each coefficient by a step size from an ISO perceptual table and round. High-frequency coefficients (which the eye doesn't see well) get large step sizes — they round to zero often, which is where compression happens.

Step 4 — **Entropy coding**: Serialize the block in zigzag order (DC first, high-frequencies last), write the DC as a delta from the previous block, then write ACs up to the last non-zero one (truncating trailing zeros). Feed this stream into an adaptive Huffman coder.

On decode: reverse every step — Huffman decode → dequantize → Inverse DCT → bilinear chroma upsample → YCbCr to RGB.

---

## 2. Image Compression Theory

### Q: What is the difference between lossy and lossless compression?

**Lossless**: Every original bit is recoverable. Example: PNG uses DEFLATE (LZ77 + Huffman). Compression ratios typically 2–5× for photos.

**Lossy**: Some information is permanently discarded. The key insight is that human perception is not perfect — we can't see high-frequency spatial detail or fine color gradations, so discarding them doesn't degrade perceived quality. JPEG at quality=75 achieves 10–50× compression with negligible perceptual loss.

DCTPress is lossy. `decode(encode(img)) ≠ img` — pixels differ, but at high quality settings the difference is below human detection threshold.

### Q: Why is JPEG so effective for photographs but bad for screenshots?

DCT assumes smooth local frequency content — photographs have gradual gradients that DCT represents efficiently. Screenshots have sharp edges, flat color regions, and text — the DCT of a sharp edge spreads energy across all frequencies, requiring many non-zero coefficients. PNG (lossless) is better for screenshots.

### Q: What is the frequency domain and why does it matter for compression?

The spatial domain represents an image as pixel intensities at (x, y) positions. The frequency domain represents it as the amplitudes of sinusoidal components at different spatial frequencies. In the frequency domain, natural images are "sparse" — most energy is in low frequencies, and high-frequency components are small or zero. This sparsity is what compression exploits: you can set small high-frequency values to zero without visibly degrading the image.

### Q: What is entropy in information theory and how does it relate to compression?

Shannon entropy H = -Σ p(x) log₂ p(x) is the theoretical minimum average bits needed to represent a source. After quantization, many DCT coefficients are zero — zero becomes the most probable symbol, so entropy is low. Huffman coding assigns shorter bit-strings to more frequent symbols, approaching the entropy lower bound.

---

## 3. DCT — The Core Algorithm

### Q: What is the DCT formula?

For an 8-point 1D DCT-II:

```
F(k) = 0.5 · α(k) · Σ_{n=0}^{7} f(n) · cos((2n+1)kπ/16)

where α(0) = 1/√2, α(k) = 1 for k > 0
```

The 2D DCT is separable: apply 1D DCT to each row, then to each resulting column. The output `F(u,v)` is mathematically equivalent to the direct 2D formula:

```
F(u,v) = 0.25 · α(u) · α(v) · Σ_{x=0}^{7} Σ_{y=0}^{7} f(x,y) · cos((2x+1)uπ/16) · cos((2y+1)vπ/16)
```

### Q: Why DCT and not FFT (Fast Fourier Transform)?

FFT produces complex-valued output with phase and magnitude. For image compression we need real-valued coefficients. The DCT (specifically DCT-II) has the additional property that it implicitly assumes even-symmetric extension at block boundaries, which reduces boundary artifacts compared to the DFT's periodic extension assumption.

Also, the DCT of natural images concentrates more energy in fewer coefficients ("energy compaction") than the DFT.

### Q: What is the DC coefficient and why is it special?

The DC coefficient `F(0,0)` is proportional to the mean pixel value of the 8×8 block. It carries the most energy and is the most perceptually important. In the encoder:

1. It is quantized with a 20%-tighter step size (finer precision = better PSNR)
2. It is delta-coded — stored as the difference from the previous block's DC value, not the absolute value. Adjacent blocks tend to have similar mean luminance, so deltas are small and Huffman-compress better.

### Q: Why does the encoder do level shifting?

Before applying DCT, pixel values [0, 255] are shifted to [-128, 127]. This centers the signal at zero, ensuring the DC coefficient reflects true block variation rather than a large positive bias. Without level shifting, the DC term would always be large and positive, wasting quantization range.

### Q: What is the separable property and how did it improve performance?

A 2D function `f(x, y) = g(x) · h(y)` is separable — you can process each dimension independently. The 2D DCT kernel is separable:

```
cos((2x+1)uπ/16) · cos((2y+1)vπ/16)
```

Because of this, instead of 8×8×8×8 = 4096 multiplications per block (O(n⁴)), two sequential 1D passes need only 2 × 8 × 8 = 128 multiplications (O(n³)). For n=8 that's a 32× reduction — this is why encode time dropped from ~800ms to ~40ms.

### Q: How is the IDCT different from the DCT?

The IDCT (DCT-III) is the inverse of DCT-II. The formula swaps the role of the summation variable:

```
f(n) = 0.5 · Σ_{k=0}^{7} α(k) · F(k) · cos((2n+1)kπ/16)
```

It uses the same precomputed cosine table — only the outer loop (over output positions vs frequency bins) is swapped.

### Q: What are blocking artifacts and how do you mitigate them?

When quantization is aggressive, each 8×8 block is reconstructed differently from its neighbors. At block boundaries the discontinuity becomes visible as a grid pattern — "blocking." Mitigations:

- Overlapping block transforms (HEVC, AV1)
- Deblocking filters applied post-decode
- Higher quality setting (less aggressive quantization)
- Larger block sizes (16×16, 32×32) — fewer boundaries

DCTPress doesn't currently apply a deblocking filter — it would improve low-quality output but wasn't in scope.

---

## 4. Quantization & Perceptual Coding

### Q: What is quantization in the context of DCT?

Each DCT coefficient is divided by a step size Q[u][v] and rounded to the nearest integer:

```
quantized[u][v] = round(F(u,v) / Q[u][v])
```

On dequantization: `F̃(u,v) = quantized[u][v] × Q[u][v]`. The error `F(u,v) - F̃(u,v)` is at most Q[u][v]/2 — this is the "quantization noise." Larger Q = more noise = smaller file.

### Q: What are the JPEG ISO Annex K quantization tables and why use them?

The standardLuminanceTable (Y channel) and standardChrominanceTable (Cb/Cr) are empirically derived from human contrast sensitivity function (CSF) measurements. Low-frequency entries are small (fine quantization = preserve detail). High-frequency entries are large (coarse quantization = discard imperceptible detail).

Example: `Q_luma[0][0] = 16` (DC, most important), `Q_luma[7][7] = 99` (highest frequency diagonal, least visible).

Using these tables means DCTPress inherits decades of perceptual research rather than inventing its own.

### Q: How does the quality parameter work?

```go
// JPEG reference formula
if quality < 50 {
    scale = 50.0 / quality    // e.g. quality=25 → scale=2.0 → double all steps
} else {
    scale = 2.0 - quality/50  // quality=75 → scale=0.5 → halve all steps
}
quantStep = clamp(round(baseTable * scale), 1, 255)
```

Quality=100 → scale=0 → all steps = 1 (minimum quantization, near-lossless).
Quality=1 → scale=50 → all steps maxed at 255 (extreme lossy).
Quality=50 → scale=1.0 → use tables as-is.

### Q: Why reduce the DC step by 20%?

The DC coefficient carries the block's mean luminance — the most energy-dense, perceptually dominant component. A 20% reduction in its quantization step doubles the number of distinct DC values representable without changing any AC coding. The effect on PSNR is ~0.08 dB improvement for a ~0.4 KB increase in file size — the best PSNR-per-byte tradeoff available within the existing architecture.

### Q: What is a deadzone quantizer and why was it ultimately not used?

A deadzone quantizer forces values in `(-threshold, +threshold)` to zero before rounding:

```
if |coefficient| < threshold × step:
    quantized = 0
else:
    quantized = round(coefficient / step)
```

This produces more zeros → shorter Huffman codes for AC coefficients. However, it trades PSNR for size. Testing showed a deadzone of 0.64×step for high-frequency coefficients increased file size by 2% but dropped PSNR by 0.26 dB — a poor trade. The final encoder uses standard round-to-nearest.

---

## 5. Entropy Coding — Huffman

### Q: How does Huffman coding work?

1. **Count frequencies** of all symbols in the input stream
2. **Build a min-heap** (priority queue) of leaf nodes, keyed by frequency
3. **Build tree**: repeatedly pop the two lowest-frequency nodes, create an internal node with their combined frequency, push it back. Repeat until one node remains (the root).
4. **Assign codes**: traverse the tree — left edge = '0', right edge = '1'. Each leaf's path from root is its code.
5. **Encode**: replace each symbol with its bit-string. More frequent symbols get shorter codes.

The resulting codes satisfy the prefix-free property: no code is a prefix of another, so the stream can be decoded unambiguously.

### Q: What is the time complexity of building a Huffman tree?

Building the frequency table: O(n) where n is the number of symbols.
Building the tree: O(k log k) where k is the number of unique symbols (each heap operation is O(log k), and there are 2k-1 nodes to process).

For DCT coefficients k is typically 50–500, so this is essentially constant relative to image size.

### Q: How is the Huffman tree serialized into the file?

Pre-order traversal with type tags:

```
Leaf node  → 'L' byte + int16 symbol (LE)
Internal   → 'I' byte + left subtree + right subtree
```

This allows reconstruction in one pass on decode. The bit-packed coefficient stream follows, with a padding byte indicating how many tail bits are padding.

### Q: What is the "single-symbol fast path"?

If every coefficient in the stream is the same value (e.g., all zeros for a blank channel), a Huffman tree can't be built — you need at least two distinct symbols to form a tree. The encoder detects this and writes `[symbolCount=1][symbol][count]` instead of building a tree, saving the overhead of tree serialization and bit-packing.

### Q: Why is adaptive (per-image) Huffman better than a fixed table?

A fixed table (like standard JPEG) is optimized for the average image. An adaptive table is built from the actual coefficient distribution of this specific image — frequent values get shorter codes, rare ones longer. For images with unusual distributions (very uniform, very detailed), adaptive coding can be 5–15% more efficient. The cost is the overhead of storing the tree in the file header.

### Q: Why didn't run-length encoding (RLE) help?

Expected hypothesis: `(zeroRun, value)` pairs would be shorter than writing many zero values individually.

Actual result: file size increased from 97 KB to 134 KB.

Root cause: the adaptive Huffman encoder had already assigned the single bit `'0'` to the zero symbol (most frequent). Writing one bit per zero is already optimal. Adding RLE pairs (two symbols per run) eliminated this advantage and added extra overhead for dense blocks where runs are short. The lesson: don't add an optimization without profiling — the existing approach had already solved the problem you thought you were solving.

### Q: Huffman vs Arithmetic coding — which is better and why?

Arithmetic coding achieves entropy within 1 bit total overhead vs Huffman's up to 1 bit per symbol. For a source where the most probable symbol has probability 0.9, Huffman must assign it 1 bit, but arithmetic coding approaches 0.15 bits — a 6× improvement on that symbol.

In practice for DCT coefficient streams, arithmetic coding gives 5–10% better compression. The downside: more complex implementation (range normalization, carry propagation), harder to debug, patented in some jurisdictions (though those patents have expired). This is why JPEG uses Huffman in its baseline profile and arithmetic coding only in the less-supported arithmetic JPEG variant.

---

## 6. Color Spaces & Chroma Subsampling

### Q: Why convert to YCbCr instead of compressing in RGB?

The human visual system has ~6× more luminance-sensitive (L cone) neurons than color-sensitive neurons. We are much more sensitive to brightness variations than color variations. YCbCr separates these signals, allowing us to aggressively compress Cb and Cr while preserving Y. If we compressed R, G, B equally, we'd waste bits protecting color information the eye doesn't need.

### Q: What is 4:2:0 chroma subsampling?

The notation `J:a:b` describes the ratio of luma to chroma samples in a 4-pixel wide, 2-row region:
- J=4: reference width (always 4)
- a: chroma samples in the first row
- b: chroma samples in the second row

4:2:0 → 2 chroma samples in row 1, 0 additional in row 2 (same as row 1). Both Cb and Cr are downsampled 2× horizontally and 2× vertically → 4× fewer samples. For a 1920×1080 image: Y = 2,073,600 samples, Cb = Cr = 518,400 samples each.

4:2:2 → halved horizontally only. 4:4:4 → full resolution (no subsampling).

### Q: What is chroma siting and why does it matter on upsample?

When downsampling 2:1, where does the reduced-resolution sample "live"? JPEG cosited siting places the chroma sample at the centre of its group of luma pixels. The upsample formula must account for this:

```
srcCoord = (targetIndex + 0.5) / scale - 0.5
```

If you use `srcCoord = targetIndex / scale` (corner-sited), the color planes are systematically shifted by half a pixel, causing visible fringing on high-contrast color edges. Switching to the correct formula improved PSNR by ~0.15 dB.

### Q: What are the BT.601 conversion coefficients and where do they come from?

ITU-R BT.601 defines the matrix for standard-definition content derived from the relative luminance sensitivity of the three CIE color matching functions:

```
Y  =  0.299·R + 0.587·G + 0.114·B
```

The weights reflect that the eye is most sensitive to green (~59%), moderately to red (~30%), and least to blue (~11%). BT.709 uses different coefficients for HDTV content; BT.2020 for UHD. DCTPress uses BT.601 (matching JPEG standard).

---

## 7. Quality Metrics — PSNR & SSIM

### Q: What is PSNR and how is it calculated?

Peak Signal-to-Noise Ratio measures reconstruction fidelity. Higher is better.

```
MSE   = (1/N) Σ (original[i] - reconstructed[i])²
PSNR  = 10 · log10(MAX² / MSE)     where MAX = 255 for 8-bit
```

Typical values: >40 dB excellent, 30–40 dB good, <30 dB noticeable degradation. PSNR is computed per channel (Y, Cb, Cr) and averaged or reported for Y only.

DCTPress at quality=75: 37.34 dB. Go stdlib JPEG: 37.51 dB. The 0.17 dB gap is below human detection threshold (~0.5 dB) and is systemic to the adaptive Huffman design choice.

### Q: Why is PSNR sometimes misleading?

PSNR assumes all errors are equally perceptually damaging — a wrong pixel in a smooth sky and a wrong pixel on a sharp edge both count equally. In reality, the human eye is less sensitive to errors in high-frequency or textured regions. A codec that systematically misses high-frequency detail will have low PSNR but may look better than one that shifts colors slightly (high PSNR).

### Q: What is SSIM and how is it better?

Structural Similarity Index (SSIM) compares images on three dimensions: luminance, contrast, and structure. It operates on local windows (11×11 pixels) and models spatial correlations:

```
SSIM(x,y) = (2μ_xμ_y + C1)(2σ_xy + C2) / ((μ_x² + μ_y² + C1)(σ_x² + σ_y² + C2))
```

SSIM = 1.0 means identical. Values > 0.97 are typically indistinguishable to humans. It correlates better with perceptual quality than PSNR, especially for blocking and blurring artifacts.

DCTPress SSIM = 0.9850 vs Go stdlib 0.9863 — practically equivalent.

### Q: What does it mean that DCTPress has better size but lower PSNR than Go stdlib?

It means DCTPress is trading a small amount of signal fidelity for a smaller file. The 2.8% size advantage comes from the adaptive Huffman fitting the coefficient distribution better than the static JPEG Huffman in some configurations. The 0.17 dB PSNR gap comes from the overhead of storing the adaptive tree header, which takes bytes that could have gone toward finer quantization.

At the perceptual level (SSIM) the two are nearly equal: 0.9850 vs 0.9863, a difference of 0.0013 — imperceptible.

---

## 8. Data Structures & Algorithms

### Q: Why use a min-heap to build the Huffman tree?

Building the tree requires repeatedly finding the two nodes with the lowest frequency and combining them. A min-heap provides O(log k) extract-min and O(log k) insert — making the O(k log k) tree construction efficient. A linear scan would be O(k²). In Go, `container/heap` implements a min-heap via a heap-ordered slice with `Push`/`Pop` interface.

### Q: What is zigzag ordering and why does it help?

An 8×8 DCT block has energy concentrated in the top-left (low frequency). Zigzag traversal visits the block along anti-diagonals, starting at [0,0] (DC) and ending at [7,7] (highest frequency). After zigzag, the 64-element sequence typically has large values at the start and many zeros at the end. The EOB (End-of-Block) marker can then truncate all trailing zeros in one symbol, saving significant space.

Without zigzag, zeros would be scattered throughout the sequence and EOB compression would be ineffective.

### Q: What is delta coding for DC coefficients?

Adjacent 8×8 blocks in the same image tend to have similar average luminance (natural images are locally smooth). Rather than storing each block's DC value absolutely, the encoder stores only the difference from the previous block's DC. These differences are small integers (often 0, ±1, ±2), which Huffman codes efficiently with 1–3 bits vs 8–12 bits for the absolute value.

### Q: How is the .dct binary format structured?

```
Offset  Size  Field
0       3     Magic: "DCT"
3       1     Version: 0x02
4       4     Width (uint32 LE)
8       4     Height (uint32 LE)
12      1     Quality (uint8)
13      1     Subsampling mode (uint8)
14      4     Y channel length in bytes (uint32 LE)
18      4     Cb channel length (uint32 LE)
22      4     Cr channel length (uint32 LE)
26      yLen  Y Huffman-encoded coefficient stream
...     cbLen Cb stream
...     crLen Cr stream
```

### Q: How do you handle images whose dimensions aren't multiples of 8?

Pad the image to the next multiple of 8 in each dimension before encoding (replicate-edge padding). After decoding the padded planes, trim back to the original dimensions. This is a standard JPEG technique — the padded region is compressed and decompressed but discarded on output.

---

## 9. Complexity Analysis

### Q: What is the time complexity of encoding one image?

Let W×H = image dimensions, B = number of 8×8 blocks = (W/8)×(H/8), n = 8.

| Step | Complexity |
|---|---|
| RGB→YCbCr | O(W·H) |
| Chroma downsample | O(W·H) |
| Padding | O(W·H) |
| Forward DCT (separable) | O(B · n³) = O(W·H·n) |
| Quantization + zigzag | O(B · n²) = O(W·H) |
| Huffman freq count | O(W·H) |
| Huffman tree build | O(k log k) ≈ O(1) (k ≤ 512) |
| Huffman encode | O(W·H) |
| **Total** | **O(W·H)** |

All O(W·H) — linear in pixel count. Constant multiplier is ~128 operations per pixel (dominated by DCT).

### Q: What is the space complexity?

Three full-size float64 planes (Y, Cb, Cr) + quantized int planes + Huffman bitstream. For a 1920×1080 image:

- Float64 plane: 1920 × 1080 × 8 bytes ≈ 16 MB
- Three planes: ~48 MB
- Compressed output: ~38 KB

Peak memory is about 50–60 MB for 1080p. The original pixel data is ~6 MB (RGBA), so the working set is ~10× input size.

### Q: Where are the hot loops?

Profiling would show DCT transform as the dominant cost: 3 channels × B blocks × 2 passes × 64 multiply-accumulates = ~300M FLOPs for 1080p. Everything else (Huffman, quantization, color conversion) is negligible by comparison.

---

## 10. Go Language Deep Dive

### Q: Why use `[blockSize][blockSize]float64` arrays instead of slices?

Value semantics: the entire 8×8 block is copied when passed to/from functions. This avoids allocation on the heap — arrays declared as local variables stay on the stack (up to the compiler's escape analysis limit). Stack allocation is ~10× faster than heap allocation and produces zero GC pressure.

Slices would require `make()`, live on the heap, and put pressure on the garbage collector during the hot DCT loop.

### Q: How does Go's escape analysis affect the DCT code?

If a local variable's address escapes the current function (passed to an interface, stored in a struct that outlives the function, etc.), Go moves it from stack to heap. By using value types (`[8][8]float64`) and passing by value, the DCT functions keep all working data on the stack. Running `go build -gcflags="-m"` would confirm no allocations in the hot path.

### Q: What is `container/heap` and how does the `heap.Interface` work?

`container/heap` provides heap operations on any type implementing:

```go
type Interface interface {
    sort.Interface     // Len(), Less(i, j int) bool, Swap(i, j int)
    Push(x interface{})
    Pop() interface{}
}
```

You implement these five methods on your slice type, then call `heap.Init()`, `heap.Push()`, `heap.Pop()`. The package performs the `heapify` and `sift-down`/`sift-up` operations. Used in Huffman tree construction.

### Q: How would you add concurrency to this encoder?

Each 8×8 block is independent — they share no mutable state after the plane is computed. The encoder could process blocks in parallel using a worker pool:

```go
var wg sync.WaitGroup
blockCh := make(chan blockJob, runtime.NumCPU())
for i := 0; i < runtime.NumCPU(); i++ {
    go worker(blockCh, &wg)
}
// send block jobs...
wg.Wait()
```

Caution: DC delta coding introduces a dependency — block N's delta depends on block N-1's DC value. This must be computed sequentially after parallel DCT+quantization is done.

Realistic speedup: ~4–6× on 8-core hardware. Encode time would drop from ~40ms to ~8ms for 1080p.

### Q: Why doesn't the server use goroutines for parallel requests?

Go's HTTP server (`net/http`) already handles each incoming request in its own goroutine automatically. No explicit concurrency management needed for request handling. Individual encode operations are single-threaded within a request — parallelizing within one encode would be the next optimization.

### Q: What does `binary.LittleEndian.Uint32` do and why LE?

`binary.LittleEndian.Uint32(b[0:4])` reads a 4-byte little-endian unsigned integer from a byte slice: `b[0] + b[1]<<8 + b[2]<<16 + b[3]<<24`. Little-endian is used because x86/ARM are natively LE — no byte-swapping overhead on the most common server hardware.

### Q: What is the difference between `int` and `int32` in Go?

`int` is architecture-sized: 64 bits on 64-bit platforms. `int32` is always 32 bits. For array indices and loop variables, `int` is appropriate — using the native integer size avoids sign extension. For wire format fields that must be exactly 32 bits regardless of platform, `uint32` is correct.

---

## 11. System Design

### Q: How would you scale this to handle 10,000 concurrent users?

**Stateless API**: Image data is currently stored in-memory per-session. Replace with S3/GCS object storage keyed by session ID. Server becomes stateless and horizontally scalable.

**Horizontal scaling**: Put a load balancer (nginx, AWS ALB) in front of N server instances. Each handles a different request.

**Worker queues**: Encoding is CPU-bound. Use a task queue (Redis + a Go worker pool, or a message broker) to decouple upload from encode. Users get a job ID immediately; they poll for completion.

**CDN**: Serve the static web UI from CDN edge nodes. Only API calls hit the origin.

**Python benchmark**: The Python subprocess model is fragile at scale. Replace with a dedicated Python microservice with gRPC or HTTP API.

### Q: How would you add caching?

Cache compressed results keyed by (image hash, quality, subsampling). If the same image is uploaded twice at the same settings, return the cached result. Use SHA-256 of the raw pixel data as the key. Store in Redis with TTL. Estimated hit rate on typical workloads: 20–30% (many users test the same stock images).

### Q: How does the Python bridge work?

`internal/python/bridge.go` runs `python3 scripts/benchmark.py <imagePath> <quality>` as a subprocess, capturing stdout as JSON. The Go server reads the JSON and returns it alongside DCTPress results.

Risks: subprocess launch adds ~200–500ms latency (Python interpreter startup). If Python is not installed, the benchmark endpoint fails gracefully — DCTPress results are still returned.

### Q: What happens if the Python process hangs?

The bridge should use `context.WithTimeout` to kill the subprocess after a deadline:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "python3", "benchmark.py", ...)
```

`exec.CommandContext` sends SIGKILL when the context is cancelled.

### Q: How would you add authentication to the API?

For a public service, add JWT Bearer token authentication as middleware:

```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if !validateJWT(token) {
            http.Error(w, "Unauthorized", 401)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

For simple internal use, an API key in the header (`X-API-Key`) is sufficient.

---

## 12. Performance & Optimization

### Q: What made the biggest performance improvement?

The separable DCT: replacing one nested loop (O(n⁴) per block) with two sequential 1D passes (O(n³)). Encode time dropped from ~800ms to ~40ms for 1080p — a 20× speedup on the DCT alone.

Second: precomputed cosines. The `math.Cos` function is expensive (involves polynomial approximation). Computing all 64 cosine values at `init()` time and reading from a lookup table eliminates ~64 transcendental function calls per block.

### Q: How would you profile this application?

```bash
# CPU profile
go test -cpuprofile=cpu.prof -bench=. ./test/benchmarks/
go tool pprof -http=:8080 cpu.prof

# Memory profile
go test -memprofile=mem.prof -bench=. ./test/benchmarks/
go tool pprof -http=:8080 mem.prof
```

The pprof web UI shows a flame graph — call stacks ranked by time. Look for wide frames (high self-time) in the DCT and Huffman code.

### Q: What SIMD optimizations are possible?

The inner loop of DCT1D is 8 multiply-accumulate operations on consecutive floats — a perfect candidate for AVX2 SIMD (8× float32 or 4× float64 per instruction). In Go, this requires `unsafe` + assembly (`.s` files), or using `golang.org/x/sys/cpu` to detect instruction sets. Realistic speedup: 2–4× on the DCT kernel alone.

Alternatively, use `math/bits` tricks for the zigzag ordering (bitmask lookup table instead of conditional branches).

### Q: How would you reduce memory usage for very large images?

Current approach loads the entire image into three float64 planes before encoding. For a 4K image (3840×2160) that's ~190 MB of working memory.

Streaming approach: process the image in horizontal strips of 8 rows at a time. This reduces peak memory from O(W×H) to O(W×8). Requires buffering DC values across strips for delta coding.

---

## 13. Testing Strategy

### Q: What types of tests does the project have?

- **Unit tests** (`test/unit/`): pure function tests. DCT round-trip (`ForwardDCT2D` followed by `InverseDCT2D` should return the original block within floating-point tolerance), Huffman encode/decode identity, quantization table generation.

- **Integration tests** (`test/integration/`): end-to-end encode → decode → compare. Assert that PSNR > 30 dB for quality=50, that decoded image dimensions match original, that decoding a corrupt file returns an error.

- **Benchmarks** (`test/benchmarks/`): `go test -bench=. -benchmem` measures encode throughput and allocations per operation.

### Q: How do you test a lossy codec?

You cannot assert bit-exact output. Instead:
1. **Round-trip PSNR threshold**: `PSNR(original, decode(encode(original))) > 30 dB`
2. **Dimension preservation**: output image is same size as input
3. **Determinism**: encoding the same image twice with same settings produces identical output
4. **Edge cases**: all-black image, all-white, single pixel, 1×8, 8×1
5. **Fuzz testing**: random byte sequences fed to the decoder should not panic — only return `error`

### Q: What is the race detector and when would you use it?

```bash
go test -race ./...
```

The Go race detector instruments memory accesses and reports data races at runtime. Use it whenever adding concurrency. It has ~5–10× performance overhead but catches bugs that are nearly impossible to find by code review. Run it in CI, not in production.

---

## 14. API & HTTP Design

### Q: Why use `http.ServeMux` instead of a router framework like Gin or Chi?

For this project's 7 endpoints, the standard library `http.ServeMux` is sufficient. No need for path parameters, middleware chains, or content negotiation at this scale. Adding a framework dependency for its own sake increases binary size and introduces a third-party failure mode.

If the API grew to 30+ endpoints with nested resources (`/api/images/:id/crops/:cropId`), a framework providing pattern matching and middleware composition would be justified.

### Q: How does `/api/download/:id` implement path parameter extraction?

`http.ServeMux` doesn't support path parameters (in Go < 1.22). The handler strips the prefix:

```go
mux.HandleFunc("/api/download/", handlers.DownloadHandler)
// In the handler:
id := strings.TrimPrefix(r.URL.Path, "/api/download/")
```

Go 1.22+ `http.ServeMux` supports `{id}` pattern matching natively.

### Q: What HTTP status codes should each endpoint return?

| Endpoint | Success | Bad request | Not found | Server error |
|---|---|---|---|---|
| POST /api/upload | 200 | 400 | — | 500 |
| POST /api/compress | 200 | 400 | 404 (unknown id) | 500 |
| GET /api/benchmark | 200 | 400 | 404 | 500 |
| GET /api/download/:id | 200 | — | 404 | 500 |
| GET /health | 200 | — | — | — |

### Q: How would you add rate limiting?

Use a token bucket per client IP:

```go
var limiters sync.Map  // map[string]*rate.Limiter

func rateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := r.RemoteAddr
        limiter, _ := limiters.LoadOrStore(ip, rate.NewLimiter(rate.Every(time.Second), 10))
        if !limiter.(*rate.Limiter).Allow() {
            http.Error(w, "Too Many Requests", 429)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

`golang.org/x/time/rate` provides the token bucket implementation.

---

## 15. Docker & Deployment

### Q: Why a multi-stage Dockerfile?

Stage 1 (`golang:1.25-alpine`): compile the Go binary. This image contains the Go toolchain (~400 MB).
Stage 2 (`python:3.11-slim`): runtime image. Only the compiled binary and Python dependencies are copied in. Final image: ~120 MB instead of ~520 MB.

`-ldflags="-w -s"` strips debug info and symbol tables from the binary, saving ~30%.

### Q: What does `CGO_ENABLED=0` do?

Disables cgo — Go's C foreign function interface. With cgo disabled, the linker produces a fully static binary with no dependency on glibc or any C library. The binary can run in a `scratch` (empty) container or on any Linux distribution regardless of glibc version. Required for Alpine-based images (which use musl libc, not glibc).

### Q: Why run as non-root in the container?

```dockerfile
RUN adduser --disabled-password --gecos '' appuser \
    && chown -R appuser:appuser /app
USER appuser
```

Container escape vulnerabilities (privilege escalation, namespace breakouts) are significantly less impactful if the process already runs as an unprivileged user. Principle of least privilege — the server process only needs to bind a TCP port and read files, not root access.

### Q: What does the HEALTHCHECK do?

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')"
```

Docker (and orchestrators like Kubernetes) periodically run this command. If it fails 3 times, the container is marked unhealthy and can be restarted or removed from load balancer rotation. The `--start-period=10s` grace period allows the Go binary and Python runtime to initialize before health checks begin.

---

## 16. Python Bridge / IPC

### Q: What are the tradeoffs of using a subprocess vs a Python C extension vs gRPC?

| Approach | Latency | Complexity | Isolation |
|---|---|---|---|
| **subprocess** (current) | ~200–500ms startup | Low | Full process isolation |
| Python C extension (cgo) | ~1ms | Very high | None |
| gRPC microservice | ~5ms per call | Medium | Network isolated |
| REST microservice | ~10ms per call | Low | Network isolated |

Subprocess is the right choice for a benchmark tool where latency doesn't matter. For production use (e.g., serving PIL thumbnails on demand), a persistent Python gRPC service would be appropriate.

### Q: How does the benchmark comparison work technically?

1. User uploads image → Go saves to `tmp/` directory
2. GET `/api/benchmark` triggers `bridge.Run("benchmark.py", imagePath, quality)`
3. Go calls `exec.Command("python3", scriptPath, imagePath, strconv.Itoa(quality))` 
4. Python script compresses with PIL and OpenCV, outputs JSON to stdout
5. Go also compresses with stdlib JPEG and DCTPress internally
6. All results merged and returned as JSON

---

## 17. Design Trade-offs & Decisions

### Q: Why 8×8 blocks? Why not 16×16 or 4×4?

8×8 is the JPEG standard, chosen for a balance of:
- **Cache behavior**: 8×8 float64 = 512 bytes, fits in L1 cache
- **Frequency resolution**: 8 distinct frequency components in each direction is sufficient for most natural image content
- **Boundary effects**: larger blocks reduce the number of block boundaries (fewer artifacts), but require more coefficients to represent complex detail
- **Complexity**: n² coefficients → 64 for 8×8 vs 256 for 16×16

AV1 and HEVC use variable block sizes (4×4 to 64×64) adaptively chosen per region.

### Q: DCT vs Wavelets — which is better?

| | DCT (JPEG) | Wavelets (JPEG 2000) |
|---|---|---|
| Artifacts | Blocking | Ringing |
| Compression | 10–50× | 20–100× |
| Complexity | Low | High |
| Progressive | Via scan order | Naturally progressive |
| Patent issues | None | Some expired |

Wavelets avoid block boundaries entirely (the transform is global), produce better compression, and support progressive/lossy-to-lossless scaling. DCT was chosen for this project because JPEG is the world's most studied codec and the ISO tables represent decades of perceptual research that would take years to replicate for wavelets.

### Q: Why store quality in the file header?

On decode, the exact same quantization table must be used as on encode. If quality were not stored, the decoder would need it as a separate parameter — forcing callers to track quality alongside the file. Embedding it makes `.dct` files self-contained: the decoder reconstructs the quantization table from the stored quality byte alone.

---

## 18. Behavioural / STAR Questions

### "Tell me about a technical challenge you faced."

**Situation**: Run-length encoding was supposed to improve compression by replacing repetitive zero sequences with (run, value) pairs.

**Task**: Implement and measure RLE for AC coefficients.

**Action**: Implemented `(zeroRun, value)` pair encoding in the encoder and decoder. Ran benchmarks comparing file sizes.

**Result**: File size increased from 97 KB to 134 KB — a 38% regression. Investigation revealed the adaptive Huffman had already given the zero symbol a 1-bit code. RLE eliminated that optimally short encoding and added extra symbol overhead for dense blocks.

**Learning**: Measure before optimizing. The existing system had already solved the problem I thought I was solving — I needed to profile first, not hypothesize.

---

### "Describe a time you improved performance."

**S**: DCT encoding took ~800ms per 1080p image — too slow for interactive use.

**T**: Find and fix the bottleneck without compromising output quality.

**A**: Profiled with `go tool pprof`. The naive 2D DCT loop was 95% of CPU time. Rewrote using the separable property (two 1D passes) + precomputed cosine table.

**R**: Encode time: 800ms → 40ms. 20× improvement. Output is bit-identical — no quality change.

---

### "How did you decide what to work on next?"

The benchmark comparison UI surfaced gaps systematically. Once I could see DCTPress vs stdlib JPEG on the same table, the priorities were obvious: size (DCTPress was 8% larger initially), PSNR (DCTPress was 0.4 dB lower initially), and speed. Each iteration was driven by the specific metric gap showing in the live comparison.

---

## 19. Trick / Gotcha Questions

### Q: Can PSNR be infinite?

Yes — if MSE = 0 (lossless round-trip, or comparing an image to itself). The `10 · log10(MAX²/0)` is mathematically undefined but conventionally reported as ∞ dB or `math.Inf(1)`.

### Q: Is the DCT invertible? What information is lost?

The DCT itself is perfectly invertible (no information loss). Information is lost only in the **quantization** step — when you divide and round, the sub-quantum fractional part is discarded and unrecoverable. The encoder → decoder pipeline is: `original → DCT (lossless) → quantize (lossy) → Huffman (lossless) → Huffman decode (lossless) → dequantize (lossless) → IDCT (lossless) → reconstruction`. The sole source of loss is quantization.

### Q: Why does quality=100 not produce a lossless image?

At quality=100, the scale factor is `2.0 - 100/50 = 0.0`. Every quantization step would be `round(base × 0) = 0`, but steps are clamped to minimum 1. So even at quality=100, all coefficients are divided by 1 and rounded — which is lossless for integers but not for the float DCT output. The DCT output is float64; rounding to the nearest integer introduces rounding error of up to ±0.5 per coefficient, which manifests as very slight differences (PSNR typically > 55 dB, visually identical).

True lossless would require storing the full float DCT coefficients with no rounding — defeating the purpose of quantization.

### Q: Could two different images produce the same compressed output?

Yes — this is a hash collision in the compression function's output space. Two blocks whose DCT coefficients round to the same quantized integers will produce identical compressed data. This is intentional: quantization deliberately discards sub-quantum differences, treating similar blocks as identical. It's not a bug.

### Q: What happens if you decode a file with a different quality setting than it was encoded with?

The dequantization table won't match the quantization table used on encode. Coefficients will be scaled by wrong factors, producing a garbled image. This is why quality is stored in the header — the decoder always uses the encoding quality, not a user-supplied one.

### Q: Is zigzag ordering always optimal?

Zigzag is optimal for the average natural image because DCT energy concentrates in the top-left. For unusual images (e.g., pure horizontal stripes), the energy might be concentrated along a single row rather than diagonally. A codec could use image-adaptive ordering, but the overhead of storing the custom order per block is rarely worth the marginal gain.

---

## 20. What Would You Do Differently?

This is an important question — shows self-awareness and growth.

1. **Start with pprof from day one.** Spent time on run-length encoding that profiling would have immediately shown was unnecessary. Instrument first, optimize second.

2. **Use arithmetic coding instead of Huffman.** The ~5–10% PSNR gap to Go stdlib JPEG is entirely attributable to entropy coding efficiency. A range coder would close it.

3. **Variable block sizes.** Fixed 8×8 is simple but suboptimal. Smooth regions could use 16×16 or 32×32 blocks (fewer block boundaries). Detailed regions could use 4×4 (more spatial precision). HEVC and AV1 do this.

4. **Progressive encoding.** Write the DC scan first, then AC coefficients in order of frequency. The browser can display a blurry-but-complete image immediately and sharpen as more data arrives. Standard JPEG supports this; it's a significant UX improvement for large images on slow connections.

5. **Persistent Python service instead of subprocess.** The 200–500ms subprocess startup is the main latency bottleneck in the benchmark flow. A persistent FastAPI service with a single `/benchmark` endpoint would reduce benchmark latency by 10–50×.

6. **Deblocking filter.** At quality < 40, blocking artifacts become visible. A simple 1D deblocking filter at block boundaries (averaging a few pixels across the boundary) would significantly improve perceived quality at low settings with minimal computational cost.

---

*Built by Shashank S — DCTPress Image Compression Engine*
