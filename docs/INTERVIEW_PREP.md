# Interview Prep — DCTPress Image Compression Engine

> **How to use this doc**: Read the Learn/ folder first, trace every algorithm in the source, then come back here. This doc is written from the **interviewer's perspective** — every question has the hidden motive and the trap listed beside it.

> ⚠️ If you are using this project for interviews without understanding the code, interviewers will detect it in 2 follow-up questions. Read the source. Understand the math. Then read this.

---

## Table of Contents

1. [Project Pitch](#1-project-pitch)
2. [How Interviewers Actually Think About This Project](#2-how-interviewers-actually-think-about-this-project)
3. [Image Compression Theory — Core Concepts](#3-image-compression-theory)
4. [DCT — The Mathematical Core](#4-dct--the-mathematical-core)
5. [Quantization & Perceptual Coding](#5-quantization--perceptual-coding)
6. [Huffman Entropy Coding](#6-huffman-entropy-coding)
7. [Color Spaces & Chroma Subsampling](#7-color-spaces--chroma-subsampling)
8. [Quality Metrics — PSNR & SSIM](#8-quality-metrics--psnr--ssim)
9. [Data Structures & Algorithms](#9-data-structures--algorithms)
10. [Complexity Analysis](#10-complexity-analysis)
11. [Go Language Deep Dive](#11-go-language-deep-dive)
12. [Concurrency — The Hardest Part](#12-concurrency--the-hardest-part)
13. [System Design](#13-system-design)
14. [Performance & Optimization](#14-performance--optimization)
15. [Testing Strategy](#15-testing-strategy)
16. [API & HTTP Design](#16-api--http-design)
17. [Docker & Deployment](#17-docker--deployment)
18. [Python Bridge / IPC](#18-python-bridge--ipc)
19. [Design Trade-offs & What Didn't Work](#19-design-trade-offs--what-didnt-work)
20. [Company-Specific Question Sets](#20-company-specific-question-sets)
21. [Real-World Conceptual Questions](#21-real-world-conceptual-questions)
22. [Live Coding Patterns to Know](#22-live-coding-patterns-to-know)
23. [Behavioural / STAR Questions](#23-behavioural--star-questions)
24. [Trap & Gotcha Questions](#24-trap--gotcha-questions)
25. [What Would You Do Differently?](#25-what-would-you-do-differently)
26. [30-Second Answers to Hard Questions](#26-30-second-answers-to-hard-questions)

---

## 1. Project Pitch

### 30-Second Version

> "DCTPress is a from-scratch image codec in Go — same core ideas as JPEG: DCT, perceptual quantization, Huffman entropy coding — but implemented from first principles with no codec libraries. It achieves 48× compression on 1080p at quality 75, comes with a benchmarking server that compares it live against Go stdlib JPEG, Python PIL, and OpenCV, and exposes the whole thing through a REST API with a visual diff UI."

### 2-Minute Walk-Through (memorize this structure)

**Problem**: A raw 1080p RGB image is 6.2 MB. You want it to be 83 KB. How?

**Step 1 — Color separation**: Convert RGB → YCbCr. Y = brightness (most important to human eye), Cb/Cr = color difference signals. Human eyes have 4× more brightness receptors than color receptors — chroma subsampling (4:2:0) halves the color resolution. Nobody sees it. Data is now half the size before any transform.

**Step 2 — DCT**: Divide each channel into 8×8 blocks. Apply the Discrete Cosine Transform. This converts pixel brightness values into *frequency amplitudes*. Natural images are smooth — almost all the signal energy compresses into the top-left corner of the 8×8 frequency grid. The high-frequency corner is mostly zeros.

**Step 3 — Quantization**: Divide each frequency coefficient by a step size from an ISO perceptual table and round. The table has large step sizes for high frequencies (where the eye is insensitive) and small step sizes for low frequencies (where the eye is sharp). High-frequency coefficients round to zero — this is where compression happens. This step is lossy and irreversible.

**Step 4 — Entropy coding**: Serialize the block in zigzag order (DC first, high-frequency last), write the DC as a difference from the previous block's DC, truncate trailing zeros with an EOB marker, then Huffman-compress the whole stream. I built an adaptive per-image Huffman tree that adapts to the actual coefficient distribution of each specific image.

On decode: Huffman decode → dequantize → Inverse DCT → bilinear chroma upsample → YCbCr to RGB. Every step is the mathematical inverse.

**Result at quality=75**: 6.2 MB → 83 KB (48×). PSNR = 33.77 dB. SSIM = 0.9924. DCTPress produces 4% smaller files than Go stdlib JPEG at the same quality.

---

## 2. How Interviewers Actually Think About This Project

Understanding the interviewer's frame is half the battle.

### What They Are Testing

| Interviewer goal | What they ask | What they're really checking |
|---|---|---|
| Do you understand what you built? | "How does the DCT work?" | Can you go 3 levels deep on request? |
| Can you reason about trade-offs? | "Why Huffman and not arithmetic coding?" | Do you understand what you gave up? |
| Do you have engineering judgment? | "What didn't work?" | Were you rigorous or just lucky? |
| Can you handle scale? | "How would you serve 10,000 users?" | Do you know what breaks at scale? |
| Is the code quality real? | "Walk me through the concurrent block loop" | Did you write it or copy it? |
| Do you know the domain? | "What's the difference between 4:2:0 and 4:4:4?" | Have you read about image codecs beyond this project? |

### Red Flags That Will Kill the Interview

- Saying "I used DCT" without being able to write the formula
- Not knowing why the separable property matters
- Not knowing what blocking artifacts are
- Saying "PSNR is always better than SSIM" (they measure different things)
- Not knowing what the `eobMarker` value `-32768` is and why it was chosen
- "I added goroutines" without explaining the DC delta serial constraint
- Confusing the Huffman *encoder* with the Huffman *code assignment* step

### The Follow-Up Ladder

Every answer should survive three follow-ups. Example:

> Q: "Why do you use 8×8 blocks?"
> A: "Cache alignment, DCT efficiency, the ISO tables are calibrated for it."
> Follow-up: "What happens if you use 16×16?"
> A: "Better energy compaction, fewer block boundaries, but more ringing at low quality and the quantization tables need recalibration."
> Follow-up: "What does AV1 do?"
> A: "Variable block sizes from 4×4 to 128×128, chosen adaptively per region by rate-distortion optimization."

---

## 3. Image Compression Theory

### Q: What is the difference between lossy and lossless compression?

**Lossless**: Every original bit recoverable. PNG uses DEFLATE (LZ77 + Huffman). Compression ratios 2–5× for photographs. Used when: legal/medical images, text, screenshots, source code.

**Lossy**: Information permanently discarded. Key insight: human perception is imperfect — the eye cannot detect high-frequency spatial detail or fine color gradations, so discarding them doesn't degrade *perceived* quality. JPEG at quality=75 achieves 10–50× with negligible perceptual loss.

**DCTPress is lossy**: `decode(encode(img)) ≠ img` — pixels differ. At high quality, the difference is below human detection threshold (PSNR > 40 dB).

### Q: Why is JPEG excellent for photographs but poor for screenshots?

DCT assumes **locally smooth content** — gradual gradients that decompose efficiently into a few low-frequency basis functions. Photographs have this. Screenshots have:
- Sharp text edges → energy spreads across all 64 DCT coefficients
- Flat color regions → good, but the blocks between them are catastrophic
- 1-pixel borders → the highest possible spatial frequency

Use PNG for screenshots, JPEG for photos. WebP handles both via a hybrid transform.

### Q: What is Shannon entropy and why does it matter here?

Shannon entropy `H = -Σ p(x) log₂ p(x)` is the theoretical minimum average bits per symbol. After quantization, DCT coefficients have heavily skewed distributions — zero is extremely common (probability 0.6–0.8 in a typical block). A source where zero has probability 0.8 has low entropy (~0.7 bits/symbol). Huffman approaches this lower bound. Without entropy coding, you'd need ~12 bits per quantized coefficient (sign + magnitude). With adaptive Huffman, you get ~1.2 bits per symbol on average.

### Q: What are the three phases of information loss in a JPEG-like codec?

1. **Chroma subsampling** — color resolution reduced 4× (spatial loss in Cb/Cr)
2. **Quantization** — DCT coefficients rounded to integers (frequency precision loss)
3. **Nothing in entropy coding** — Huffman is lossless

Only steps 1 and 2 are lossy. The compression from Huffman is a free bonus on top.

### Q: What is the frequency domain?

The spatial domain represents an image as pixel intensities at (x, y) positions. The frequency domain represents it as amplitudes of sinusoidal components at different spatial frequencies. Natural images are **sparse in the frequency domain** — most energy in low frequencies, high-frequency amplitudes are small. Compression exploits this: set small high-frequency values to zero, store only the non-zero ones.

> **Real-world analogy**: A spreadsheet is the spatial domain (value at each cell). A Fourier analysis of a business's revenue is the frequency domain (seasonal cycles, weekly patterns, trend). The trend and seasonality capture 95% of the information in far fewer numbers than the daily raw data.

---

## 4. DCT — The Mathematical Core

### Q: Write the DCT formula.

1D DCT-II of length 8:

```
F(k) = (1/2) · α(k) · Σ_{n=0}^{7} f(n) · cos((2n+1)kπ/16)

α(k) = 1/√2   for k=0
α(k) = 1       for k ≥ 1
```

2D DCT (separable): apply 1D DCT to each row, then to each resulting column. Mathematically equivalent to the direct 2D formula but O(N³) instead of O(N⁴).

**The inverse (IDCT-II)**:

```
f(n) = (1/2) · Σ_{k=0}^{7} α(k) · F(k) · cos((2n+1)kπ/16)
```

**Interviewer trap**: They may ask "what does `(2n+1)` mean?" → It represents the center of the n-th pixel interval (half-sample offset), making the DCT an analysis of pixel *centers* rather than *edges*. This eliminates the edge discontinuity that creates Gibbs phenomenon in DFT.

### Q: Why DCT and not FFT?

| Property | DFT | DCT-II |
|---|---|---|
| Output | Complex (real + imaginary) | Real-valued only |
| Boundary assumption | Periodic extension | Even-symmetric extension |
| Energy compaction for images | Moderate | Excellent |
| Gibbs phenomenon at block edges | Yes | Minimal |

For image compression:
1. We need real-valued coefficients (no imaginary part needed)
2. Even-symmetric extension is a better model for image blocks (smooth continuation at edges)
3. DCT concentrates more energy in fewer coefficients for natural images — empirically proven

### Q: What is the DC coefficient? Why is it special?

`F(0,0)` = the mean luminance of the block multiplied by a normalization factor. It carries the most perceptual energy of any single coefficient — a wrong DC value makes the entire 8×8 block appear the wrong brightness.

Special treatment in DCTPress:
1. **Quantized with a 20% tighter step size** (`value × 0.80`) — better precision
2. **Delta-encoded** — stored as difference from previous block's DC. Adjacent blocks have similar mean luminance (natural images are locally smooth), so deltas are small integers that Huffman compresses to 1–3 bits

### Q: What is the separable property and what did it do for performance?

The 2D DCT kernel `cos((2x+1)uπ/16) · cos((2y+1)vπ/16)` factors into two independent 1D functions. So instead of computing all 8⁴ = 4096 products per block (the naive direct formula), you run two sequential 1D passes of 2 × 8² = 128 products — a **32× reduction** in operations.

Impact: encode time dropped from ~800ms to ~40ms for 1080p. The naive implementation was the first working version; separable DCT was the first major optimization.

### Q: Why precompute cosines?

`math.Cos` in Go requires a polynomial approximation and costs ~50 ns per call. For a 1080p image with 3 channels:

```
3 channels × (1920×1080)/64 blocks × 64 cosine lookups = 6,220,800 lookups per encode
```

Without precomputation: 6.2M × 50 ns = **310 ms just for cosines**.
With the 8×8 lookup table: 6.2M × ~1 ns (array read) = **6 ms**.

The table is computed once at package `init()` time and shared across all encodes.

### Q: What are blocking artifacts?

When quantization is aggressive, each 8×8 block is reconstructed with different average brightness/texture from its neighbors. At block boundaries, discontinuities become visible as a grid pattern. The frequency cause: quantization removes the high-frequency AC coefficients that normally ensure smooth transitions at block boundaries.

**Mitigations**:
- Higher quality setting (smaller step sizes)
- Deblocking filter applied post-decode (H.264, HEVC do this)
- Overlapping transforms (reduce boundary effect by design)
- Variable block sizes (fewer boundaries in smooth regions)

DCTPress has no deblocking filter — a known limitation at quality < 40.

### Q: Why level-shift before DCT?

Pixel values [0, 255] → shifted to [-128, 127] by subtracting 128.

Without level shift: the DC coefficient for a bright block might be `F(0,0) ≈ 200 × 8 × 8 × normalization ≈ 1600`. The quantization table entry for DC at quality=75 is ~6. So the quantized value is ~267 — a large integer that Huffman codes inefficiently and that delta-encoding across blocks doesn't compress well (large absolute values → large deltas).

With level shift: DC values center around 0, deltas are small, Huffman assigns short codes. One of the simplest but highest-impact design decisions in JPEG.

---

## 5. Quantization & Perceptual Coding

### Q: Walk me through how quantization works step by step.

```
1. DCT coefficient:  F(3, 5) = 47.83 (a mid-frequency coefficient)
2. Quantization table entry: Q[3][5] = 87 (at quality=75, from luma table)
3. Scaled:  Q_scaled[3][5] = round(87 × 0.5) = 43   [quality=75 → scale=0.5]
4. DC tuning: only applies to [0][0], not this coefficient
5. Quantize: round(47.83 / 43) = round(1.113) = 1
6. On decode: 1 × 43 = 43  (instead of original 47.83)
7. Error: 4.83 — permanently lost
```

### Q: What are the ISO Annex K tables and why use them?

Two empirically derived 8×8 matrices for luma (Y) and chroma (Cb/Cr) channels, from human contrast sensitivity function (CSF) experiments. The CSF describes how sensitive the human eye is to sinusoidal gratings at different spatial frequencies and orientations.

Key values from the luma table:
- `Q[0][0] = 16` — DC (most sensitive, fine quantization)
- `Q[0][1] = 11` — adjacent horizontal (finer than DC — horizontal is perceptually critical)
- `Q[7][7] = 99` — maximum frequency diagonal (coarsest — eye ignores it)

These are not invented — they represent the result of decades of visual psychophysics research. Using them gives DCTPress the same perceptual optimization foundation as commercial JPEG codecs.

### Q: How does the quality parameter map to step sizes?

```python
# JPEG reference formula
if quality < 50:
    scale = 50.0 / quality       # q=1 → 50×, q=25 → 2×, q=49 → ~1×
else:
    scale = 2.0 - quality / 50.0 # q=50 → 1×, q=75 → 0.5×, q=100 → 0×

step = clamp(round(base_table[u][v] × scale), 1, 255)
```

The two formulas are designed to meet at quality=50 (scale=1, use base tables as-is) and diverge symmetrically.

Clamping to [1, 255]:
- Minimum 1: prevents division by zero in quantization
- Maximum 255: JPEG stores table entries in 1 unsigned byte

### Q: Why reduce the DC step by 20%?

`value × 0.80` applied only to `table[0][0]`.

The DC coefficient carries the block's mean luminance — the dominant perceptual component. Error in DC makes the entire 8×8 block appear wrong-brightness, which is immediately visible. A 20% tighter step gives ~0.08 dB PSNR improvement at the cost of ~0.4 KB per 1080p image. This is the best PSNR-per-byte improvement available without changing the overall pipeline.

Tested alternatives:
- 50% tighter: disproportionate file size increase
- 10% tighter: marginal PSNR gain
- 20% tighter: optimal empirical trade-off

### Q: What is a deadzone quantizer? Why was it rejected?

A deadzone expands the "round to zero" region: instead of rounding values in `(-0.5 × step, +0.5 × step)` to zero, a deadzone rounds values in `(-d × step, +d × step)` to zero (typically d = 0.64 for HEVC).

In theory: more zeros → better Huffman compression.
In DCTPress testing: file size increased 97 KB → 134 KB when deadzone applied to high-frequency coefficients.

Root cause: the adaptive Huffman had *already* assigned 1-bit codes to zero (the most frequent symbol). The deadzone reduced the diversity of non-zero values, making the remaining non-zero coefficients harder to code efficiently. The structure the deadzone was supposed to exploit had already been exploited by the Huffman tree.

**Lesson**: Measure before optimizing. The system had already solved the problem the deadzone was supposed to solve.

---

## 6. Huffman Entropy Coding

### Q: Build a Huffman tree from scratch. Walk me through the algorithm.

```
Symbols: {0: freq 70, 1: freq 15, -1: freq 10, -32768: freq 5}

Step 1 — Min-heap (priority queue ordered by frequency):
  [(-32768, 5), (-1, 10), (1, 15), (0, 70)]

Step 2 — Merge two lowest (5, 10):
  Internal node: freq=15
  Heap: [(1,15), (internal,15), (0,70)]

Step 3 — Merge two lowest (15, 15):
  Internal node: freq=30
  Heap: [(internal2, 30), (0, 70)]

Step 4 — Merge last two:
  Root: freq=100

Resulting tree:
          root(100)
         /          \
    inner2(30)       0(70)
    /        \
inner1(15)   1(15)
 /      \
-32768(5) -1(10)

Codes (left=0, right=1):
  0      → "1"       (1 bit, 70% of stream)
  1      → "01"      (2 bits)
  -1     → "000"     (3 bits)
  -32768 → "001"     (3 bits — the EOB marker)
```

**Time complexity**: O(k log k) where k = unique symbols. Each of the 2k-1 heap operations is O(log k).

**Why min-heap?** We always merge the two *rarest* nodes. A min-heap gives O(log k) extract-min. A sorted array scan would be O(k) per merge → O(k²) total. For k=500 symbols: 500² = 250,000 vs 500 × log(500) ≈ 4,500 — a 55× difference.

### Q: How does adaptive Huffman differ from static JPEG Huffman?

| | Static JPEG | Adaptive (DCTPress) |
|---|---|---|
| Tables | Pre-defined for average image | Built fresh per image per channel |
| File overhead | 0 (tables implicit) | ~100–500 bytes for tree header |
| Compression for typical photo | Good | Similar |
| Compression for unusual images | Suboptimal | Optimal for this image |
| Decode speed | Fast (tables pre-loaded) | Fast (tree pre-built before decode) |

For images with unusual statistics (synthetic graphics, medical images, flat screenshots), adaptive can be 5–15% more efficient. The tree header overhead is amortized over the full image — for anything > 100×100, adaptive wins.

### Q: Why did run-length encoding make things worse?

Expected: storing (numZeros, nextNonZero) pairs would be shorter than writing many individual zeros.

Actual: file 97 KB → 134 KB (38% regression).

Root cause analysis:
1. Adaptive Huffman had assigned `0` the code `"0"` — literally 1 bit
2. Writing 10 consecutive zeros: 10 × 1 bit = 10 bits
3. With RLE: (10, nextVal) = two symbols, each needing 3–8 bits = 6–16 bits
4. Net result: RLE was slower to encode than individual 1-bit zeros

The system had already optimally solved the sparse-zero problem through the Huffman tree. Adding RLE eliminated the advantage.

### Q: What is a prefix-free code and why does Huffman produce one?

A prefix-free (prefix code) is a set of codewords where no codeword is a prefix of another. This enables unambiguous decoding: as you read bits, when you find a valid codeword, you know the symbol is complete — no lookahead needed.

Huffman's tree structure guarantees prefix-free codes: all symbols are leaves. A leaf's code (path from root) cannot be a prefix of another leaf's code because you'd have to pass through an internal node, not stop at a leaf.

The decoder reads bits one by one, traversing the tree. When it reaches a leaf, it emits that symbol and returns to the root.

### Q: Huffman vs Arithmetic Coding — which is better?

Arithmetic coding achieves the entropy lower bound within a total overhead of 2 bits (vs Huffman's up to 1 bit per symbol overhead). For a symbol with probability 0.9, arithmetic coding approaches log₂(1/0.9) = 0.152 bits, while Huffman must assign at least 1 bit.

For DCT coefficient streams with heavy zero probability (~0.7): arithmetic coding would assign ~0.515 bits to zero; Huffman assigns 1 bit. That's ~2× better coding for the dominant symbol.

**Why not use it?** Arithmetic coding requires:
- Range normalization with carry propagation
- Complex bit-exact implementation
- Patents (expired, but still complex)

The goal of DCTPress was to understand the pipeline, not squeeze the last 5–10%. Huffman is the right pedagogical choice.

### Q: Explain the single-symbol fast path.

If every coefficient in the stream is the same value (e.g., a blank channel where all coefficients are 0), you cannot build a Huffman tree — you need at least 2 distinct symbols to form a binary tree.

Special encoding:
```
[symbolCount=1] [symbol (int16)] [count (uint32)]
```
Total: 4 + 2 + 4 = 10 bytes, regardless of how many symbols.

Without this path, you'd need a degenerate "tree" of one node and a special decoder case. The fast path is simpler and more compact.

---

## 7. Color Spaces & Chroma Subsampling

### Q: Why YCbCr instead of compressing RGB directly?

The human visual system has:
- ~6 million L cones (luminance, brightness-sensitive)
- ~1 million M+S cones (color, but far fewer)
- About 6× more luminance sensitivity than color sensitivity spatially

RGB channels are highly correlated (for any neutral gray, R = G = B). Compressing correlated channels wastes bits on redundant information. YCbCr decorrelates:
- Y captures all brightness (high sensitivity) → must be stored at full precision
- Cb, Cr capture color deviation → can be stored at reduced resolution

The mathematical benefit: after YCbCr conversion, the DCT of the Y channel is much more energy-compact (fewer non-zero coefficients) than the DCT of R, G, or B, because Y is already a good approximation of the image structure.

### Q: What is 4:2:0 chroma subsampling precisely?

The `J:a:b` notation describes sampling in a reference block of J pixels wide, 2 rows tall:
- J = horizontal luma reference width (always 4 in modern notation)
- a = chroma samples in row 1
- b = chroma samples in row 2

| Mode | a | b | Chroma resolution | Data saved |
|---|---|---|---|---|
| 4:4:4 | 4 | 4 | Full | 0% |
| 4:2:2 | 2 | 2 | Half width | 33% |
| 4:2:0 | 2 | 0 | Half width, half height | 50% |

For 1920×1080 at 4:2:0:
- Y: 1920×1080 = 2,073,600 samples
- Cb: 960×540 = 518,400 samples
- Cr: 960×540 = 518,400 samples
- Total: 3,110,400 vs 6,220,800 (50% reduction before any DCT)

### Q: What is JPEG cosited chroma siting?

When downsampling chroma 2:1, the chroma sample can be placed at different positions relative to luma samples. JPEG uses "cosited" siting: chroma sample is aligned with the top-left luma sample of each 2×2 group.

On upsample, the mapping from target pixel to source coordinate must account for this:

```go
srcCoord = (targetPixel + 0.5) / scale - 0.5
```

Without this formula (using `srcCoord = targetPixel / scale`), the chroma plane is shifted by half a pixel. On high-contrast color edges, this causes visible color fringing — a systematic error. Correcting to cosited siting gives ~0.15 dB PSNR improvement for free.

### Q: What are the BT.601 coefficients and where do they come from?

```
Y = 0.299·R + 0.587·G + 0.114·B
```

These coefficients match the relative luminance sensitivity of the three CIE color matching functions — derived from spectral sensitivity measurements of the human retina under standard illumination. Green contributes the most (0.587) because the eye's peak sensitivity is in the green wavelength range (~555 nm). Blue contributes least (0.114) — the short-wavelength S cones are the fewest.

BT.601 is for standard definition. BT.709 (HDTV): `0.2126, 0.7152, 0.0722`. BT.2020 (HDR/UHD): `0.2627, 0.6780, 0.0593`. DCTPress uses BT.601, matching the JPEG standard.

### Q: What is bilinear interpolation and why is it better than nearest-neighbor for chroma upsampling?

**Nearest-neighbor**: each upsampled pixel copies the nearest source pixel. A 2× upsampled chroma plane has 2×2 blocks of identical color. At color boundaries, this creates step discontinuities visible as color banding.

**Bilinear**: the interpolated value is a weighted average of the 4 nearest source pixels:

```
result = tl*(1-fx)*(1-fy) + tr*fx*(1-fy) + bl*(1-fx)*fy + br*fx*fy
```

Where (fx, fy) is the fractional distance from the top-left source pixel. This produces smooth color transitions, eliminating banding at color edges.

Quantified improvement in DCTPress: **+0.15 to +0.3 dB PSNR** with zero change in compressed file size. It's a decode-time improvement that costs ~5% more decode time in exchange for meaningfully better quality.

---

## 8. Quality Metrics — PSNR & SSIM

### Q: Calculate PSNR given MSE.

```
MSE = mean squared error per channel sample
PSNR = 10 × log10(255² / MSE)
     = 10 × log10(65025 / MSE)

Example: MSE = 16.4 (DCTPress at quality=75)
PSNR = 10 × log10(65025 / 16.4)
     = 10 × log10(3965.5)
     = 10 × 3.598
     = 35.98 dB
```

**Interpretation table**:

| PSNR | Human perception |
|---|---|
| > 40 dB | Visually indistinguishable from original |
| 35–40 dB | Minor artifacts visible on close inspection |
| 30–35 dB | Visible artifacts, acceptable for most uses |
| < 30 dB | Significant degradation |
| +∞ | Identical images (MSE = 0) |

### Q: When is PSNR misleading?

Three known failure modes:

1. **Blurring**: A Gaussian blur has small average pixel error (high PSNR) but looks terrible subjectively. PSNR doesn't penalize blur more than sharp-edge artifacts.

2. **Spatial shift**: Shifting an image by 1 pixel gives terrible PSNR (every pixel is "wrong") but looks identical to humans.

3. **Color shift**: A slight hue shift across the entire image can have high PSNR per-pixel but looks obviously wrong to the eye.

This is why SSIM was invented — it measures *structural patterns* rather than per-pixel differences.

### Q: Explain the SSIM formula — what do each of the three terms measure?

```
SSIM(x, y) = L(x,y) × C(x,y) × S(x,y)
```

Over a local window of pixels:

1. **Luminance** `L = (2μₓμᵧ + C₁) / (μₓ² + μᵧ² + C₁)`: How close are the mean brightnesses? Stable when means are equal.

2. **Contrast** `C = (2σₓσᵧ + C₂) / (σₓ² + σᵧ² + C₂)`: How close are the standard deviations? Captures whether local contrast is preserved.

3. **Structure** `S = (σₓᵧ + C₃) / (σₓσᵧ + C₃)`: Normalized covariance — are the spatial patterns in x and y correlated? This is the key perceptual component: even if brightness shifts slightly, if the local texture pattern is the same, the image "looks the same."

The stability constants `C₁ = (0.01×255)²`, `C₂ = (0.03×255)²` prevent division by zero for flat regions (σ ≈ 0) and are the standard Wang et al. 2004 values.

### Q: Why is SSIM computed on luminance only?

The human visual system is primarily sensitive to luminance structure (brightness edges, textures) rather than chromatic structure. Computing SSIM on all three channels would weight color errors equally to luminance errors, which overweights the perceptually less important channels. Computing on Y alone correlates better with perceived quality.

This also makes SSIM faster — three floating-point array passes vs one.

### Q: DCTPress vs Go stdlib JPEG — how do you interpret the metrics?

DCTPress at quality=75, 1920×1080:
- File: 83.4 KB vs 86.9 KB (4% smaller)
- PSNR: 33.77 dB vs 33.84 dB (0.07 dB difference)
- SSIM: 0.9924 vs 0.9934 (0.0010 difference)
- Encode: 179 ms vs 29 ms (6× slower)

**Interpretation**: DCTPress produces a slightly smaller file with imperceptibly different quality (0.07 dB PSNR is below human detection threshold of ~0.5 dB). The SSIM difference of 0.001 is also perceptually irrelevant. The 6× speed penalty is real and significant — DCTPress is not competitive for real-time encoding.

The 4% size advantage comes from the adaptive Huffman fitting the specific image's coefficient distribution better than JPEG's fixed Huffman tables. It comes at the cost of storing the tree header.

---

## 9. Data Structures & Algorithms

### Q: What data structures are used and why?

| Data Structure | Where | Why |
|---|---|---|
| `[8][8]float64` fixed array | DCT blocks | Stack-allocated, no GC pressure, type-safe |
| `[64]int` fixed array | Zigzag sequence | Fixed-size, avoids per-block heap allocation |
| `[][]float64` 2D slice | Image planes | Height × width, dynamic dimensions |
| `[]int` slice | Coefficient stream | Dynamic, append-heavy |
| `nodeHeap` (slice + heap ops) | Huffman tree build | O(log k) insert/extract |
| `map[int]int` | Frequency count | Hash map, O(1) increment |
| `map[int]string` | Code table | Symbol → bit string |
| `map[string]imageEntry` | Image store | Session ID → image |
| `sync.RWMutex` | Concurrent map access | Multiple readers, exclusive write |
| `chan blockWork` (buffered) | Worker pool | Backpressure-free work distribution |

### Q: How does the zigzag ordering table work?

```go
var zigzagOrder = [64][2]int{
    {0,0}, {0,1}, {1,0}, {2,0}, {1,1}, {0,2}, ...
}
```

A hard-coded lookup table of 64 (row, col) pairs defining the traversal order. To serialize a block:

```go
for index, position := range zigzagOrder {
    output[index] = block[position[0]][position[1]]
}
```

This is O(64) = O(1) regardless of image size. The zigzag was computed analytically from the anti-diagonal traversal pattern.

**Why not compute it dynamically?** The pattern is fixed, deterministic, and only 128 bytes. Computing it at runtime would add unnecessary complexity. Hard-coded lookup tables for small fixed patterns are the correct engineering choice.

### Q: Explain delta coding for DC values.

```
Block 0 DC:  95  → store 95
Block 1 DC:  93  → store 93 - 95 = -2
Block 2 DC:  94  → store 94 - 93 = +1
Block 3 DC:  91  → store 91 - 94 = -3
```

Decoders: cumulative sum. `prevDC = 0` initially. `dc = prevDC + delta; prevDC = dc`.

Why it compresses better: natural images are locally smooth. Adjacent blocks have similar mean luminance. Deltas concentrate near zero. The Huffman tree assigns very short codes to small deltas. Instead of encoding a 12-bit absolute value (range ±2048), you encode a 2–5 bit delta. A 3–6× bit reduction for the DC component alone.

### Q: What is the EOB marker and why is -32768?

`eobMarker = -32768 = math.MinInt16` — the minimum value of a signed 16-bit integer.

After the last non-zero AC coefficient in the zigzag sequence, the EOB marker is written. The decoder reads values until it sees -32768 and fills remaining positions with zeros.

**Why -32768 specifically?** No valid quantized DCT coefficient can have this value. The maximum possible DCT coefficient for 8-bit pixel values (range [-128, 127]) with identity quantization (step size = 1) is bounded by `|F(u,v)| ≤ 8 × 127 × max_cosine_product ≈ 1016`. The quantized value at step size = 1 is at most ±1016, well within int16's range of [-32767, 32767]. -32768 is the one int16 value that can never be a valid coefficient — making it safe as a sentinel.

---

## 10. Complexity Analysis

### Q: What is the total time complexity of encoding?

| Stage | Input | Complexity | Notes |
|---|---|---|---|
| RGB → YCbCr | W×H pixels | O(W·H) | 3 operations per pixel |
| Chroma subsampling | W×H | O(W·H) | Average 2×2 regions |
| Block extraction | per block | O(64) = O(1) | |
| Forward DCT (separable) | per block | O(2·8²) = O(128) | Two 1D passes |
| Quantization + zigzag | per block | O(64) | |
| Total DCT phase | B blocks | O(B × 128) = O(W·H) | |
| Huffman freq count | all coefficients | O(W·H) | |
| Huffman tree build | k unique symbols | O(k log k) ≈ O(1) | k ≤ 512 in practice |
| Huffman encode | all coefficients | O(W·H) | |
| **Overall** | | **O(W·H)** | Linear in pixel count |

The entire pipeline is O(W·H) — linear in image size. The constant factor is ~200–400 operations per pixel.

### Q: What is the space complexity?

Peak memory during encode of a W×H image:

| Buffer | Size | Example (1080p) |
|---|---|---|
| Original RGBA | W×H×4 | 8.3 MB |
| Y plane (float64) | W×H×8 | 16.6 MB |
| Cb plane | (W/2)×(H/2)×8 | 4.15 MB |
| Cr plane | (W/2)×(H/2)×8 | 4.15 MB |
| Huffman bitstream | ~W×H×2 bytes | ~4 MB |
| **Peak** | | **~40 MB** |

Peak is about 6–7× the raw file size. For 4K (3840×2160), peak is ~160 MB. Streaming/tiling approaches reduce this.

### Q: Where are the hot loops and what can you do about them?

Profile-driven answer:
1. **DCT transform** (70% of CPU): `forwardDCT1D` inner loop — 8 multiply-accumulates. SIMD-optimizable (AVX2: 4 float64 per instruction → 2× speedup).
2. **Huffman encode** (20%): string concatenation for bit buffer. Replace with bit-level operations for 3–5× speedup.
3. **Color conversion** (5%): can be vectorized with SIMD for ~4× speedup.
4. **Quantization, zigzag, metrics** (5%): marginal, not worth optimizing first.

For DCTPress, the parallel worker pool provided ~4× speedup on 8-core hardware at negligible code complexity cost.

---

## 11. Go Language Deep Dive

### Q: Why `[8][8]float64` instead of a slice?

Fixed-size arrays in Go are **value types** — they live on the stack when declared as local variables (verified by escape analysis). Benefits:

1. **Zero allocation**: no `make()`, no GC pressure, no GC pause
2. **Cache locality**: 8×8×8 = 512 bytes, fits entirely in L1 cache (typically 32–64 KB)
3. **Type safety**: `[7][8]float64` is a different type from `[8][8]float64` — compiler catches dimension bugs
4. **Copy semantics**: `b := a` copies all 64 floats — no aliasing bugs in parallel code

A slice version would require `make([][]float64, 8)` + 8 inner `make([]float64, 8)` — 9 heap allocations per block, creating GC pressure across millions of blocks.

### Q: What is escape analysis and how does it apply here?

Go's compiler analyzes whether a variable's reference can "escape" to a scope that outlives the creating function. If it can, the variable moves to the heap. If not, it stays on the stack.

The DCT functions use value receiver returns (`[8][8]float64`) — the fixed arrays are returned by copy, not by pointer. The caller stores the return value on its own stack frame. No pointer escapes → no heap allocation.

Check with: `go build -gcflags="-m" ./internal/core/dct/`

### Q: Explain `sync.RWMutex` — when to use vs plain `sync.Mutex`.

`sync.RWMutex` has two modes:
- `Lock()` / `Unlock()`: exclusive write access (one goroutine at a time)
- `RLock()` / `RUnlock()`: shared read access (multiple goroutines simultaneously)

Use when: reads far outnumber writes. The image store has many concurrent GET requests (reads) and fewer uploads (writes). `RWMutex` allows all reads to proceed concurrently without blocking each other — only a write blocks all readers.

Use plain `sync.Mutex` when: reads and writes are roughly equal, or when simplicity outweighs the marginal performance gain of RW.

### Q: What is `container/heap` and how does the interface work?

`container/heap` implements heap operations on any type satisfying:

```go
type Interface interface {
    sort.Interface          // Len(), Less(i, j int) bool, Swap(i, j int)
    Push(x interface{})     // add element
    Pop() interface{}       // remove and return element at Len()-1 after heap ops
}
```

You implement these 5 methods on your type, then call `heap.Init()`, `heap.Push()`, `heap.Pop()`. The package handles the sift-up/sift-down operations.

`Less(i, j int) bool { return h[i].frequency < h[j].frequency }` makes it a **min-heap** — the root is always the element with the lowest frequency, which is exactly what Huffman tree construction requires.

### Q: What does `defer` do and how is it used here?

`defer` schedules a function call to execute when the surrounding function returns — via normal return, `return` statement, or `panic`. LIFO order if multiple defers.

In DCTPress, used for:
1. `defer file.Close()` — ensures file is closed even if later code returns early with an error
2. `defer wg.Done()` — ensures the WaitGroup counter is decremented when a goroutine exits
3. `defer imageStoreMu.Unlock()` — ensures mutex is always released

Without `defer`, every error return path must manually call cleanup — easy to miss, causing resource leaks or deadlocks.

### Q: What is the difference between `fmt.Errorf("...%w", err)` and `fmt.Errorf("...%v", err)`?

`%w` wraps the original error: `errors.Is()` and `errors.As()` can unwrap and inspect the original error type.
`%v` embeds the error as a string: the original error type is lost.

In DCTPress, errors are wrapped with `%w` throughout (`"compression failed: %w"`, `"Y channel Huffman decode failed: %w"`), so a caller can check `errors.Is(err, someSpecificError)` even after multiple wrapping levels.

---

## 12. Concurrency — The Hardest Part

### Q: Walk me through the parallel block processing design.

```go
// Pre-allocate result slice — each goroutine writes to a different index
processed := make([]blockResult, totalBlocks)

// Buffered channel — main goroutine enqueues all work without blocking
workChan := make(chan blockWork, totalBlocks)

// Launch fixed number of workers (= runtime.NumCPU())
var wg sync.WaitGroup
for w := 0; w < numWorkers; w++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for work := range workChan {
            // DCT + quantize + zigzag — all independent per block
            processed[work.idx].zigzag = zigzagFlattenFixed(quantizeBlock(ForwardDCT2D(extractBlock(...))))
        }
    }()
}

// Send all work
for idx := 0; idx < totalBlocks; idx++ {
    workChan <- blockWork{idx, row, col}
}
close(workChan)  // signals workers to stop when queue is drained
wg.Wait()        // wait for all goroutines to finish
```

**Why this pattern is safe**: Each goroutine writes `processed[work.idx]` — a different index. Multiple goroutines writing different indices of a slice is safe without a mutex in Go (they write to different memory locations).

**Why buffered channel with capacity = totalBlocks?** Lets the main goroutine enqueue all work instantly without blocking. Workers pull at their own pace. No goroutine waits for the main goroutine to unblock.

### Q: Why is DC delta coding serial and not parallel?

DC delta coding produces `delta[n] = dc[n] - dc[n-1]`. Block n's delta depends on block n-1's DC value. This is a **sequential dependency** — you cannot compute delta[5] without knowing delta[4]'s result, which depends on delta[3], etc.

This is why the encoder has two phases:
1. **Parallel**: DCT + quantize + zigzag (independent per block)
2. **Serial**: DC delta coding + EOB truncation (sequential)

The parallel phase does the heavy CPU work (~95% of time). The serial phase is lightweight (~1 addition per block).

### Q: What is a goroutine leak and how do you prevent one here?

A goroutine leak happens when a goroutine is blocked waiting for something that never arrives and is never garbage-collected.

Scenario: if the main goroutine panics after enqueuing work but before calling `close(workChan)`, workers would be stuck in `for work := range workChan` forever.

Prevention: `close(workChan)` always executes because it's in the main goroutine after the send loop — if the send loop panics, the defer mechanisms in the workers (which call `wg.Done()`) ensure the WaitGroup doesn't deadlock, but the goroutines would still be stuck.

Production-quality solution: use `context.WithCancel()` and select on both `workChan` and `ctx.Done()`:

```go
go func() {
    defer wg.Done()
    for {
        select {
        case work, ok := <-workChan:
            if !ok { return }
            // process work
        case <-ctx.Done():
            return  // context cancelled → clean exit
        }
    }
}()
```

### Q: What is a data race? Give an example from this codebase.

A data race occurs when two goroutines access the same memory location concurrently and at least one access is a write, without synchronization.

Example if `imageStoreMu` were removed:

```
Goroutine A (upload handler):
    imageStore["img_1"] = imageEntry{...}  // WRITE

Goroutine B (compress handler, simultaneous):
    entry := imageStore["img_1"]            // READ
```

Without the mutex: Go's map internally reorganizes during growth. Concurrent read+write to a growing map can cause a `concurrent map read and map write` panic.

The race detector (`go test -race`) would catch this by instrumentating every memory access.

### Q: Explain the `context.WithTimeout` pattern in the Python bridge.

```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
cmd := exec.CommandContext(ctx, "python3", scriptPath, ...)
```

`context.WithTimeout` creates a context that automatically cancels after 60 seconds. `exec.CommandContext` monitors the context — if cancelled, it sends SIGKILL to the subprocess.

`defer cancel()` ensures the context's internal timer goroutine is cleaned up even if the function returns early (e.g., if Python errors immediately). Without `defer cancel()`, the timer goroutine leaks.

---

## 13. System Design

### Q: How would you scale DCTPress to 10,000 concurrent users?

**Current bottlenecks at scale**:
1. In-memory session store — single server, no sharing between instances
2. CPU-bound encode — one encode saturates one core
3. Python subprocess — stateful, not shared

**Scaled architecture**:

```
Client
  │ HTTPS
  ▼
Load Balancer (AWS ALB)
  │  Round-robin / least-connections
  ├── Server 1  ┐
  ├── Server 2  ├── Stateless API servers (horizontally scaled)
  └── Server N  ┘
        │
        ├── S3/GCS — image blob storage (replaces in-memory store)
        │           Key: session_id → image bytes
        │
        ├── Redis — session metadata + result cache
        │          Key: (image_hash, quality, subsampling) → result
        │
        ├── SQS/Kafka — work queue for CPU-bound encodes
        │
        └── Worker Pool — separate fleet for encoding
              │ poll queue, encode, write to S3
              └── Python gRPC service — persistent, warm Python runtime
```

**Key changes**:
- Stateless servers — no in-memory state, any request goes to any server
- Work queue — decouple upload (fast) from encode (slow). User gets job ID immediately, polls for completion
- Result cache — same image at same quality = cache hit, instant response
- Dedicated worker fleet — scale encode workers independently of API servers

### Q: Design an image resizing and compression service for a social media platform.

**Requirements to clarify**:
- Scale: how many uploads/sec? Reads/sec?
- Latency: synchronous during upload or async?
- Quality: perceptual quality threshold?
- Formats: JPEG only? WebP? AVIF?

**Design**:

```
1. Upload → object storage (raw, original resolution preserved)
2. Async job triggered → encoding worker
3. Worker generates: thumbnail (200×200), medium (800px), large (1600px)
   - Each at WebP + JPEG for browser compatibility
   - Quality calibrated by SSIM threshold: SSIM ≥ 0.95
4. Encoded variants stored in CDN-attached storage
5. Read path: CDN edge serves pre-encoded variants
   - Cache hit ratio > 99% for popular content
   - Miss: pull from storage, cache at edge
6. Format selection: server-side via Accept header (WebP vs JPEG)
```

**Trade-offs to discuss**:
- Synchronous encoding: better UX, limits throughput
- Async encoding: higher throughput, shows placeholder during processing
- Quality vs size: SSIM threshold vs fixed quality number
- Format: WebP saves 25–35% vs JPEG at same quality, but requires format negotiation

### Q: What breaks first if the server gets 100× more traffic?

In order:

1. **Python subprocess** — process startup latency and single-threaded execution. Bottlenecks at ~10 concurrent benchmark requests.
2. **In-memory image store** — grows unboundedly, OOM at ~500 large images.
3. **CPU for encoding** — 100 concurrent 1080p encodes would saturate 4 cores. Queue forms.
4. **Go HTTP server** — goroutine-per-request is lightweight (~8KB stack). The stdlib server handles thousands of concurrent connections.
5. **Port 8080** — TCP connection limit (tunable, not a real bottleneck).

---

## 14. Performance & Optimization

### Q: What was the biggest single performance improvement?

Separable DCT — changing from O(N⁴) to O(N³) per block:

```
Before: direct 2D formula — 8⁴ = 4096 multiply-adds per block
After: two 1D passes — 2 × 8² = 128 multiply-adds per block
Ratio: 32×
```

Combined with precomputed cosines (eliminating `math.Cos` calls), encode time: **800ms → 40ms for 1080p**. A 20× wall-clock improvement.

### Q: How would you profile this code?

```bash
# Generate CPU profile
go test -cpuprofile=cpu.prof -bench=BenchmarkEncode1080p ./test/unit/dct/
go tool pprof -http=:8081 cpu.prof   # opens browser flame graph

# Generate memory profile
go test -memprofile=mem.prof -bench=BenchmarkEncode1080p ./test/unit/dct/
go tool pprof -http=:8082 mem.prof

# Inline trace for goroutine scheduling
go test -trace=trace.out -bench=. ./test/unit/dct/
go tool trace trace.out
```

The flame graph shows call stacks ranked by self-time. Wide frames = hot code. For DCTPress, expect `forwardDCT1D` to be the widest frame.

### Q: What SIMD optimizations are possible?

The `forwardDCT1D` inner loop:

```go
for k := 0; k < 8; k++ {
    sum := 0.0
    for n := 0; n < 8; n++ {
        sum += x[n] * precomputedCosines[k][n]  // 8 multiply-accumulates
    }
}
```

This is exactly what AVX2 was designed for: `_mm256_fmadd_pd` does 4 double-precision FMAs per instruction. 8 values → 2 AVX2 instructions vs 8 scalar. Expected speedup: 2–4× on the DCT kernel.

Implementation in Go requires `.s` assembly files — complex but well-documented. The `golang.org/x/sys/cpu` package lets you detect AVX2 availability at runtime for a safe fallback.

### Q: How would you reduce memory for 4K+ images?

Streaming/tiling approach:
- Process the image in horizontal strips of 8 rows at a time
- Strip height = blockSize (8) — exactly one block row
- Per-strip memory: O(W × 8) instead of O(W × H)

Constraints:
- DC delta coding requires the DC of the last block in the previous strip — carry a single `prevDC` value across strips
- Chroma subsampling for 4:2:0 requires pairs of rows — process 16 luma rows per strip (8 chroma rows)
- Huffman must see the entire coefficient stream to build the optimal tree — either two-pass (first pass collects stats, second encodes) or use an approximated table

For 4K (3840×2160): current peak ~160 MB → tiled approach: ~5 MB peak. Significant for embedded or mobile deployments.

---

## 15. Testing Strategy

### Q: How do you test a lossy codec correctly?

You cannot assert bit-exact output. Instead:

1. **Mathematical properties** (unit): `IDCT(DCT(x)) == x` to floating-point precision (1e-9 tolerance), DCT of constant block has zero AC components
2. **Round-trip quality thresholds**: `PSNR(original, decode(encode(original, q=75))) > 30 dB` — fails only if something is fundamentally wrong
3. **Dimension preservation**: output image dimensions == input dimensions
4. **Format validity**: header magic bytes, version, length fields consistent with actual data
5. **Edge cases**: all-black, all-white, 8×8 minimum, non-multiple-of-8 dimensions, high-frequency patterns
6. **EOB alignment** (regression): dense blocks at Q100 don't corrupt the next block's DC delta position
7. **Huffman round-trip**: `Decode(Encode(symbols)) == symbols` for all symbol types
8. **Fuzz testing**: arbitrary bytes fed to `Decode()` must return `error`, never panic

### Q: What does the race detector catch that code review misses?

Code review can catch obvious race conditions but misses:
- Goroutines accessing a map without a mutex when the code path looks sequential but actually isn't
- A mutex protecting write but not read of a field
- A channel send from one goroutine and close from another goroutine without coordination

The race detector instruments every memory access at runtime and reports races with the specific goroutines and code locations involved. It's the only reliable way to find races in concurrent code.

Cost: 5–10× runtime overhead, 5–10× memory overhead. Run in CI, not production.

### Q: What is the `httptest` package and why is it used in integration tests?

`httptest.NewRequest` creates an `*http.Request` without network I/O.
`httptest.NewRecorder` implements `http.ResponseWriter`, capturing status code, headers, body.

This lets you call handler functions directly, testing the full HTTP logic without starting a real server:

```go
req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
rec := httptest.NewRecorder()
handlers.UploadHandler(rec, req)
// now inspect rec.Code, rec.Body
```

Advantages: fast (no TCP round-trip), isolated (no port conflicts), testable without network permissions.

---

## 16. API & HTTP Design

### Q: Why `http.ServeMux` instead of Gin, Echo, or Chi?

For 7 endpoints with no path parameters (except the manual strip for `/api/download/`): the stdlib is sufficient. Adding a framework:
- Adds a third-party dependency (supply chain risk)
- Adds binary size (~500 KB for Gin)
- Adds learning curve for new contributors
- Solves no real problem the stdlib can't handle

If the API grew to 30+ endpoints with nested path parameters, middleware chains, or content negotiation, a framework would be justified.

In Go 1.22+, `http.ServeMux` natively supports path parameters (`/api/download/{id}`) and method-based routing — the gap with frameworks has largely closed.

### Q: What is `multipart/form-data` and how does it work?

`multipart/form-data` is an HTTP encoding for binary data (files). The request body is divided into "parts" separated by a boundary string. Each part has headers (content-disposition, content-type) followed by the data.

```
Content-Type: multipart/form-data; boundary=----FormBoundary123

------FormBoundary123
Content-Disposition: form-data; name="image"; filename="photo.jpg"
Content-Type: image/jpeg

[binary JPEG bytes]
------FormBoundary123--
```

`request.ParseMultipartForm(maxBytes)` parses this, making individual parts available via `request.FormFile("image")`. The `maxBytes` parameter sets the in-memory buffer size — data beyond this is spilled to disk.

### Q: What HTTP status codes does each endpoint return and why?

| Endpoint | 200 | 400 | 404 | 413 | 500 |
|---|---|---|---|---|---|
| POST /api/upload | success | bad multipart, unsupported format | — | file > 10 MB | decode error |
| POST /api/compress | success | bad JSON, quality out of range | unknown image_id | — | encode failed |
| POST /api/benchmark | success | bad JSON | unknown image_id | — | internal error |
| GET /api/download/:id | success | — | unknown dl_id | — | — |
| GET /api/export | success | bad format param | id not found | — | re-encode failed |
| GET /health | always 200 | — | — | — | — |

---

## 17. Docker & Deployment

### Q: What is a multi-stage Docker build and why use it here?

Multi-stage build uses multiple `FROM` statements. Each stage can copy files from earlier stages.

```dockerfile
# Stage 1: Build (Go toolchain: ~300MB, not in final image)
FROM golang:1.25-alpine AS builder
RUN go build -ldflags="-s -w" -o /imagecompressor ./cmd/server/

# Stage 2: Runtime (only the binary + Python runtime)
FROM python:3.11-slim
COPY --from=builder /imagecompressor ./
```

Result: final image is ~120 MB (Python + binary) instead of ~450 MB (Python + Go toolchain + source).

`-ldflags="-s -w"`: `-s` removes symbol table, `-w` removes DWARF debug info. Reduces binary by ~30%. Trade-off: no meaningful stack traces in production (add a logging/tracing layer separately).

### Q: What does `CGO_ENABLED=0` do and why is it needed for Alpine?

CGO allows Go to call C code. With `CGO_ENABLED=0`:
- The Go toolchain uses pure-Go implementations of all standard library components
- The resulting binary is **statically linked** — no dependencies on system shared libraries
- Can run on any Linux regardless of glibc version

Alpine Linux uses musl libc instead of glibc. A binary compiled with CGO enabled and linked against glibc will fail on Alpine with "no such file or directory" for glibc. `CGO_ENABLED=0` eliminates this dependency entirely.

### Q: Why run as non-root in a container?

Container escape vulnerabilities (e.g., namespace breakouts) give the attacker the same privileges the container process has. Root in the container → root on the host if the escape succeeds.

The server process needs: bind TCP port >1024 (non-root allowed), read files from disk, write to `/tmp`. It does not need root. `USER appuser` enforces the principle of least privilege at the OS level.

### Q: How does the Docker HEALTHCHECK work?

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')"
```

Every 30 seconds, Docker runs this command. If it returns non-zero 3 consecutive times, the container is marked `unhealthy`. Orchestrators (Kubernetes, ECS) can then restart it or remove it from load balancer rotation.

`--start-period=10s`: Docker doesn't count failures during the first 10 seconds. This prevents the container being killed before the server finishes initializing.

The health check uses Python (instead of `curl`) because Python is already in the image — no additional tool needed.

---

## 18. Python Bridge / IPC

### Q: What are the tradeoffs of subprocess vs gRPC for Python integration?

| Approach | Latency | Complexity | Failure isolation |
|---|---|---|---|
| **Subprocess** (current) | 200–500ms startup | Low (shell out + JSON parse) | Full — crash kills subprocess only |
| gRPC persistent service | ~5ms per call | Medium (proto, service, client) | Good — network boundary |
| REST microservice | ~10ms per call | Low | Good |
| CGO + C Python extension | ~0.1ms | Very high | None — crash kills Go server |

For a benchmark tool: subprocess is correct. For production thumbnail generation: persistent gRPC service.

### Q: How does the Go server handle Python script failures?

Three failure modes:

1. **Python not found**: `IsAvailable()` returns false → Python benchmarks silently skipped
2. **Script crash (ImportError, etc.)**: `cmd.Run()` returns non-nil error → stderr captured and logged → that codec's result omitted from response
3. **Script hangs**: 60-second context timeout → SIGKILL sent to subprocess → error returned → result omitted

In all cases: Go benchmarks (DCTPress + stdlib JPEG) still run and return results. The API response may have 2 or 4 results, never 0.

### Q: Why pass the image as a temp PNG file rather than stdin?

Options considered:

1. **Temp PNG file** (current): PIL and OpenCV both have robust file readers; numpy array loading from file is simple; no streaming protocol needed
2. **stdin bytes**: PIL can read from BytesIO; requires base64 encoding for JSON or raw binary over stdin protocol — more fragile
3. **Shared memory**: most complex; overkill for a benchmark tool

PNG was chosen over JPEG for the temp file to avoid double-compression — if JPEG were used for transfer, the Python script would measure quality relative to a pre-compressed image, making the comparison unfair. PNG is lossless: the Python script sees the exact same pixel values as Go.

---

## 19. Design Trade-offs & What Didn't Work

### Q: Why 8×8 blocks?

1. **Cache alignment**: 8×8 float64 = 512 bytes — fits in L1 cache (typical 32–64 KB)
2. **DCT efficiency**: 8-point DCT has special fast algorithms (AAN butterfly: 29 multiplications vs 128 for separable naive)
3. **ISO table calibration**: Annex K tables are specifically psychophysically calibrated for 8×8
4. **Perceptual scale**: 8 pixels at arm's length subtends about 1–2 arcminutes — the threshold where spatial frequency sensitivity drops

Larger blocks (HEVC uses up to 64×64 CTUs): better energy compaction in smooth regions, fewer boundaries, but more memory, more complex rate-distortion optimization.

### Q: What didn't work — tell me about a failed optimization.

**Run-length AC encoding**:
- Hypothesis: (numZerosBefore, value) pairs would shrink the sparse AC stream
- Implementation: modified encoder and decoder, tested on 5 representative images
- Result: file size +37% (97 KB → 134 KB)
- Root cause: adaptive Huffman already assigned 1-bit code to zero; RLE eliminated that and added symbol overhead

**Deadzone quantization**:
- Hypothesis: forcing near-zero AC coefficients to 0 via deadzone (0.64×step) would increase compression
- Result: 2% smaller file, 0.26 dB PSNR loss — net negative
- Root cause: the perceptual table already has appropriate step sizes; adding a deadzone double-quantizes

Both failures reinforce the same lesson: the system had already solved the problem. Measure, don't hypothesize.

### Q: Why Go instead of C++ or Rust?

| Language | DCT speed | Concurrency model | Dev speed | Binary size |
|---|---|---|---|---|
| **Go** (chosen) | ~40ms 1080p | Goroutines, channels | Fast | ~8 MB |
| C++ | ~8ms 1080p | Threads + mutexes | Slow | ~2 MB |
| Rust | ~10ms 1080p | async/await or threads | Medium | ~3 MB |

Go gave:
- Goroutine-based parallelism that maps cleanly to the block-independent encoding pattern
- Zero-dependency stdlib (HTTP, JSON, image codecs)
- Escape analysis keeping hot-path data on stack
- 40ms encode time — fast enough for interactive use

C++ would be 5× faster for raw DCT. For this use case (interactive web tool, not real-time encoder), 40ms is acceptable. Choosing Go over C++ here is a defensible engineering trade-off, not a compromise.

---

## 20. Company-Specific Question Sets

### Google (Algorithm + System Design Focus)

- **Coding**: "Implement Huffman encoding given a list of (symbol, frequency) pairs. Return the code table."
- **Algorithm**: "Explain the DCT energy compaction property. Why does natural image energy concentrate in low-frequency coefficients?"
- **Design**: "Design an image compression API that serves 1 billion requests/day. Where are the bottlenecks?"
- **Go**: "What's the difference between a goroutine and an OS thread? How does Go schedule goroutines?"
- **Scale**: "Your image store grows unboundedly. How do you add eviction without breaking in-flight requests?"
- **Deep dive**: "Walk me through every byte in your .dct file format. Why those fields in that order?"

### Meta (Product Sense + Execution)

- **Product**: "Instagram stores billions of images. What compression strategy would you use? How do you balance quality vs storage cost?"
- **A/B test**: "You're testing a new compression algorithm. What metrics do you track? What constitutes a win?"
- **Reliability**: "Your encoding service has a 0.1% error rate. How do you find and fix the root cause?"
- **Scale**: "You need to re-compress 5 billion existing images with a new codec. How do you migrate without downtime?"
- **Go concurrency**: "I see you use a channel-based worker pool. Why a buffered channel specifically? What happens if the buffer is too small?"

### Netflix (Domain Knowledge + Trade-offs)

- **Domain**: "Netflix generates personalized artwork for each title for each user. How would you design the compression pipeline?"
- **Perceptual quality**: "For movie trailers, which matters more — PSNR or SSIM? Why?"
- **Format**: "When would you use JPEG vs WebP vs AVIF? Design a format selection strategy."
- **Progressive**: "Explain progressive JPEG. Why is it important for slow connections?"
- **Trade-off**: "Encoding is CPU-expensive. Should it happen at upload time or request time? What changes between the two?"

### Apple (Correctness + Privacy)

- **On-device**: "Your codec needs to run on an iPhone. How do you adapt it for ARM with NEON SIMD?"
- **Memory**: "An iPhone has 4GB RAM shared with the OS. How do you handle 48MP camera images in your pipeline?"
- **Privacy**: "Images should not leave the device. How does your codec design change if all processing must be on-device?"
- **Testing**: "How do you test that your codec produces bit-identical output across iOS, macOS, and visionOS?"
- **Quality**: "A user complains that compressed images look 'banded'. What is the root cause and how do you fix it?"

### Cloudflare (Edge + Scale)

- **Edge compute**: "Your codec needs to run at the CDN edge. What are the constraints? (No GPU, limited memory, cold starts)"
- **Format negotiation**: "How do you serve WebP to browsers that support it and JPEG to those that don't? Implement the logic."
- **Caching**: "Two users request the same image at different quality settings. How do you cache efficiently?"
- **Latency**: "The codec takes 40ms for 1080p. For edge deployment with a 100ms latency budget, is that acceptable? What would you change?"
- **Bandwidth**: "Your edge node has 100 Mbps upload bandwidth. How many concurrent 1080p encode+serve requests can it handle?"

### Adobe (Perceptual Quality + Professional Use)

- **Color accuracy**: "A professional photographer complains your compression shifts colors. What's wrong and how do you fix it?"
- **Color profiles**: "Your encoder uses BT.601. A client sends a BT.2020 wide-gamut image. What happens? What should happen?"
- **Lossless**: "Design a lossless mode for DCTPress. What changes in the pipeline?"
- **Metadata**: "JPEG stores EXIF data (camera model, GPS, etc.). Your .dct format doesn't. How would you add it?"
- **Print quality**: "A printer requires PSNR > 50 dB. What quality setting achieves this and what is the file size?"

---

## 21. Real-World Conceptual Questions

These require domain knowledge beyond the project itself.

### Q: How does HEVC (H.265) improve on JPEG?

| Feature | JPEG | HEVC |
|---|---|---|
| Block size | Fixed 8×8 | Variable 4×4 to 64×64 (CTU) |
| Transform | DCT-II only | DCT + DST adaptively |
| Intra prediction | None (block-independent) | 35 directional modes |
| Entropy coding | Huffman | CABAC (context-adaptive arithmetic) |
| Compression vs JPEG | 1× | 2–4× at same quality |

HEVC's key additions: intra prediction (predicts current block from neighboring decoded blocks before DCT), adaptive block sizes (large blocks for smooth regions), CABAC (arithmetic coding adapted to symbol context).

### Q: What is progressive JPEG and how does it differ from baseline?

**Baseline JPEG**: coefficients are stored sequentially block-by-block. To display a partial download, you get the top N rows at full quality.

**Progressive JPEG**: coefficients are stored in *scans* — DC scan first (coarse image), then low AC frequencies, then high AC. A partial download gives a complete image at low resolution that sharpens as more data arrives. Better UX for slow connections.

Implementation: multiple Huffman passes over all blocks, each pass adding higher-frequency detail. The coefficients are the same — only the order changes.

### Q: What is WebP and how does it compare to JPEG?

WebP uses a different transform (DCT for lossy, LZ77+Huffman for lossless) with additional tools:
- **Transform units**: can span multiple 8×8 blocks (reduces blocking)
- **Adaptive block quantization**: different quantization per block region
- **Color transform**: uses a learned color predictor
- **Arithmetic entropy coding**: better than Huffman

Results: 25–35% smaller than JPEG at equivalent perceptual quality. Supports both lossy and lossless in one format. Also supports alpha channel transparency.

Limitation: historically slower to encode than JPEG (libwebp is complex). Modern hardware encoders close the gap.

### Q: What is AVIF and why is it the future of web images?

AVIF (AV1 Image File Format) uses the AV1 video codec's intra-frame encoding for still images. AV1 features:
- Variable block sizes up to 128×128
- Up to 64 directional intra prediction modes
- Overlapped block motion compensation (OBMC) for smooth transitions
- Constrained directional enhancement filter (CDEF) deblocking
- Arithmetic coding with 4 entropy contexts

Results: 50% smaller than JPEG at same quality, 30% smaller than WebP. Supports HDR, wide gamut (BT.2020), 10-bit color depth.

Limitation: very slow to encode (10–100× slower than JPEG). Hardware encoders are emerging in recent CPUs.

### Q: What is the difference between intra and inter prediction?

**Intra prediction** (HEVC, AV1, used in still images): predicts a block from already-decoded neighboring blocks *within the same frame*. Reduces the signal that needs to be DCT-transformed.

**Inter prediction** (video only): predicts a block from a corresponding block in a previously decoded frame (motion compensation). This is why video codecs achieve 50–100× compression — most of the image doesn't change frame to frame.

JPEG has neither — each 8×8 block is encoded from raw pixel values. Adding intra prediction (as HEVC does for still images via HEVC-MSP, and as WebP does via intra-frame prediction) would be a major improvement to DCTPress.

### Q: What is rate-distortion optimization?

In codecs with variable-cost decisions (block size selection, quantization parameter, prediction mode), the encoder must choose the option that minimizes distortion for a given bit budget:

```
Optimal choice = argmin(Distortion + λ × Rate)
```

Where λ (Lagrange multiplier) is calibrated to the target bitrate. At each decision point (block size, QP, intra mode), the encoder evaluates all options, estimates the bits each would consume, and picks the best distortion-per-bit option.

DCTPress doesn't do RDO — it uses fixed 8×8 blocks, fixed quality-scaled tables, and no adaptive decisions per block. Adding RDO is the single biggest algorithmic improvement available.

---

## 22. Live Coding Patterns to Know

These are representative coding problems you may be asked to solve during the interview.

### Pattern 1: Huffman Tree

```go
// Given: map[rune]int of character frequencies
// Return: map[rune]string of Huffman codes

type hNode struct {
    char  rune
    freq  int
    left  *hNode
    right *hNode
}

type heap []*hNode
func (h heap) Len() int            { return len(h) }
func (h heap) Less(i, j int) bool  { return h[i].freq < h[j].freq }
func (h heap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *heap) Push(x interface{}) { *h = append(*h, x.(*hNode)) }
func (h *heap) Pop() interface{}   { old := *h; n := old[len(old)-1]; *h = old[:len(old)-1]; return n }

func buildHuffman(frequencies map[rune]int) map[rune]string {
    h := &heap{}
    for ch, freq := range frequencies {
        heap.Push(h, &hNode{char: ch, freq: freq})
    }
    heap.Init(h)
    for h.Len() > 1 {
        left := heap.Pop(h).(*hNode)
        right := heap.Pop(h).(*hNode)
        heap.Push(h, &hNode{freq: left.freq + right.freq, left: left, right: right})
    }
    root := heap.Pop(h).(*hNode)
    codes := map[rune]string{}
    var assign func(n *hNode, code string)
    assign = func(n *hNode, code string) {
        if n.left == nil { codes[n.char] = code; return }
        assign(n.left, code+"0")
        assign(n.right, code+"1")
    }
    assign(root, "")
    return codes
}
```

### Pattern 2: Worker Pool

```go
func processImages(images []image.Image, quality int) []Result {
    results := make([]Result, len(images))
    jobs := make(chan int, len(images))
    
    var wg sync.WaitGroup
    for w := 0; w < runtime.NumCPU(); w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := range jobs {
                results[i] = compress(images[i], quality)
            }
        }()
    }
    
    for i := range images { jobs <- i }
    close(jobs)
    wg.Wait()
    return results
}
```

### Pattern 3: Rate Limiter (Token Bucket)

```go
type Limiter struct {
    tokens chan struct{}
    ticker *time.Ticker
}

func NewLimiter(rps int) *Limiter {
    l := &Limiter{
        tokens: make(chan struct{}, rps),
        ticker: time.NewTicker(time.Second / time.Duration(rps)),
    }
    go func() {
        for range l.ticker.C {
            select {
            case l.tokens <- struct{}{}: // add token
            default:                     // bucket full, drop token
            }
        }
    }()
    return l
}

func (l *Limiter) Allow() bool {
    select {
    case <-l.tokens: return true
    default:         return false
    }
}
```

### Pattern 4: Concurrent Map with Expiry (in-memory cache)

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]cacheItem
}

type cacheItem struct {
    value   interface{}
    expires time.Time
}

func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    item, ok := c.items[key]
    if !ok || time.Now().After(item.expires) {
        return nil, false
    }
    return item.value, true
}

func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = cacheItem{value: value, expires: time.Now().Add(ttl)}
}
```

---

## 23. Behavioural / STAR Questions

### "Tell me about a significant technical challenge."

**S**: The first DCT implementation used the direct 2D formula — O(N⁴) operations per block. For a 1080p image, encode took 800ms.

**T**: Make it fast enough for interactive use (target < 100ms) without changing output quality.

**A**: Profiled with `go tool pprof` — DCT was 95% of CPU time, as expected. Rewrote using the separable property: two sequential 1D passes instead of the direct 2D formula. Added a precomputed cosine lookup table to eliminate `math.Cos` calls. Verified mathematical equivalence via the round-trip unit test.

**R**: Encode time: 800ms → 40ms (20×). Output is mathematically identical — not an approximation, exact same result.

### "Describe a time you had to reverse a decision."

**S**: Initially implemented run-length encoding for AC coefficients, expecting it to significantly reduce file size by eliminating long zero runs.

**T**: Measure whether RLE actually helps.

**A**: Added RLE to encoder and decoder. Ran benchmarks on 10 representative images. File size went up 38% consistently, not down. Investigated: the adaptive Huffman had already assigned 1-bit codes to zero — RLE eliminated that advantage. Reverted the change, documented the finding in the README.

**R**: Saved 500 lines of unnecessary code. Learned: measure before implementing, especially when the existing system might have already solved the problem.

### "How did you decide what to prioritize?"

The benchmark comparison table made priorities visible immediately. Once I could see DCTPress vs stdlib JPEG side-by-side with file size, PSNR, SSIM, and encode time, the gaps were obvious and ordered:

1. File size was 8% *larger* than stdlib → adaptive Huffman was the lever
2. PSNR was 0.4 dB lower → DC step tuning and bilinear upsampling closed this
3. Encode time was 6× slower → parallel block processing brought it to 4× slower

Data-driven prioritization, not gut feel.

### "Tell me about something you built that failed."

The deadzone quantizer. Theory: forcing near-zero high-frequency coefficients to zero via a deadzone would produce more zeros → shorter Huffman codes → smaller file.

Testing: 2% smaller file, 0.26 dB PSNR loss. Net negative.

Root cause: the ISO Annex K tables already encode human sensitivity at each frequency. High-frequency coefficients already have large step sizes precisely because those coefficients are perceptually unimportant. Adding a deadzone double-penalizes the same coefficients.

Learning: perceptual coding decisions made 40 years ago in ISO committees are not easily improvable with local heuristics. Understand the foundation before trying to improve it.

---

## 24. Trap & Gotcha Questions

### "Is the DCT reversible?"

**Trap**: say "yes" → follow-up: "then why does the image change after encode+decode?"

**Correct answer**: The DCT itself is perfectly invertible — `IDCT(DCT(x)) = x` exactly (verified by the round-trip unit test to 1e-9 tolerance). The source of loss is **exclusively quantization**: when you divide a float DCT coefficient by a step size and round to an integer, the fractional part is permanently discarded. DCT (lossless) → quantize (lossy) → Huffman (lossless) → Huffman decode (lossless) → dequantize (lossless) → IDCT (lossless). One lossy step in a lossless pipeline.

### "Does quality=100 produce a lossless image?"

**No**. At quality=100, scale=0, all table entries become `round(base × 0) = 0`, clamped to minimum 1. Step size = 1 for every coefficient. The quantized value = `round(F(u,v) / 1) = round(F(u,v))`. The DCT output is `float64` — rounding to the nearest integer introduces up to ±0.5 error per coefficient. True lossless would require storing DCT coefficients as exact floats — defeating the purpose.

Practical PSNR at quality=100: typically > 55 dB — visually indistinguishable but technically not identical.

### "Could two different images compress to the same .dct output?"

**Yes** — this is by design, not a bug. Two 8×8 blocks whose DCT coefficients all round to the same quantized integers produce identical compressed data. The quantizer deliberately treats inputs within the same "bin" as equivalent. It's not a collision problem — it's the codec intentionally discarding sub-quantum distinctions.

### "What happens if you decode with a different quality than was used to encode?"

The dequantization uses the wrong step sizes — every coefficient gets scaled by the wrong factor. The image is garbled: some blocks will be too bright, some too dark, textures will be wrong scale. This is why quality is stored in the 1-byte header field at offset 13 — the decoder always uses the encoding quality, never a user-supplied override.

### "Is PSNR sufficient to compare two codecs?"

**No**. PSNR measures per-pixel squared error, which doesn't model human perception:
1. PSNR doesn't penalize blur more than sharpness error (blur can look fine perceptually)
2. PSNR doesn't account for spatial frequency sensitivity (eye more sensitive to mid-frequency errors)
3. PSNR treats all channels equally (eye less sensitive to chroma errors)

You need SSIM (or MS-SSIM, VMAF, LPIPS) alongside PSNR for a complete quality comparison. DCTPress reports both.

### "Why is the EOB marker -32768 and not, say, 0xDEAD?"

The marker must be a value that **cannot appear as a valid quantized DCT coefficient**. The range of valid quantized values is bounded by the DCT output range divided by the minimum step size (1):

```
Max DCT output for 8-bit pixels ≈ 8 × 127 × 1 ≈ 1016
Valid quantized range: [-1016, 1016]
```

`-32768` (= `math.MinInt16`) is outside [-1016, 1016] and fits in a signed int16. `0xDEAD` = 57005 in unsigned, but as int16 = -8531 — also outside the valid range, so it would work. But `-32768 = math.MinInt16` is a natural sentinel: it's the extreme value of the int16 range, easy to remember, universally recognized as a sentinel value in DSP code.

### "Why do you precompute 64 cosines? Why not 8?"

The cosine table is indexed as `precomputedCosines[k][n]` — all combinations of frequency index `k` and position index `n`, both in [0, 7]. That's 8×8 = 64 unique values.

If only 8 values were stored: `cosines[k] = cos(k × π/16)`, you'd lose the dependency on `n` (the spatial index) — you couldn't evaluate `cos((2n+1)kπ/16)` without also knowing `n`. You need both `k` and `n` to look up, so you need a 2D table.

---

## 25. What Would You Do Differently?

This question tests self-awareness and engineering maturity. Have a real answer, not a humble-brag.

### 1. Arithmetic coding instead of Huffman

Arithmetic coding achieves ~5–10% better compression with no quality change. It's more complex (range normalization, carry propagation) but would close the 4% size gap with Go stdlib JPEG entirely — DCTPress would be strictly better on all metrics. The Huffman choice was pedagogical, not optimal.

### 2. Intra prediction before DCT

HEVC and WebP use intra prediction: predict each block from neighboring decoded blocks before applying the DCT. The DCT then transforms the *prediction residual* (which is small) instead of the raw pixel values. This dramatically reduces the number of significant DCT coefficients — better energy compaction, smaller file at same quality. Would be the highest-impact compression improvement.

### 3. pprof from day one

Time was spent implementing run-length encoding that profiling would have shown was unnecessary in 30 seconds. The measurement-first discipline was learned from the RLE failure, not applied before it.

### 4. Rate-distortion optimization

Allow block-level quality variation — smooth regions get more aggressive quantization; detail regions get finer quantization. This would produce significantly better quality-per-byte at the cost of more complex encoding.

### 5. Progressive mode

Write DC coefficients for all blocks first, then low-frequency ACs, then high-frequency ACs. The decoder can render a complete blurry image from partial data and sharpen as more arrives. Significant UX improvement for large images on slow connections, zero change to compression ratio.

### 6. Deblocking filter

At quality < 40, 8×8 block boundaries are visible. A simple 1D filter averaging a few pixels across each boundary would substantially improve perceived quality at low settings. The entire JPEG standard has deblocking filters as an extension for exactly this reason.

---

## 26. 30-Second Answers to Hard Questions

For when the interviewer wants a quick take, not a lecture.

**"What's the most important insight in image compression?"**
> The human eye has 4× more luminance receptors than color receptors, and is insensitive to high-frequency spatial detail. A codec that exploits this can discard most of an image's information while preserving its visual appearance.

**"Why does JPEG use 8×8 blocks?"**
> Cache alignment, DCT efficiency, and psychophysical calibration of the ISO quantization tables. AV1 uses variable block sizes up to 128×128 — JPEG made the right trade-off for its era.

**"What's the difference between PSNR and SSIM?"**
> PSNR measures per-pixel mathematical error. SSIM measures structural similarity — local luminance, contrast, and pattern correlation. PSNR is fast and universal. SSIM correlates better with what humans actually perceive.

**"Why Huffman and not arithmetic coding?"**
> Huffman is simpler to implement correctly and sufficient for this use case. Arithmetic coding gains 5–10% better compression at significantly higher implementation complexity. The trade-off was pedagogical value over compression optimality.

**"What's a goroutine leak?"**
> A goroutine blocked on a channel or syscall that will never unblock, consuming ~8KB stack permanently. Prevented by always having a termination condition — either a close(channel), context cancellation, or explicit done signal.

**"How would you scale this to handle 10M images/day?"**
> Stateless API servers behind a load balancer. Object storage (S3) for images. Work queue (SQS/Kafka) for CPU-bound encodes. Separate worker fleet. Result cache keyed by image hash + quality. Python as a persistent gRPC service, not a subprocess.

---

*Built by Shashank S — DCTPress Image Compression Engine*
*If you're using this project in interviews — trace every line of code before reading this file. The interviewer will know if you didn't.*
