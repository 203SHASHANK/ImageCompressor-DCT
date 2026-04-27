# Phase 03 — The DCT Algorithm

> *The Discrete Cosine Transform is the mathematical heart of DCTPress and JPEG. This phase covers what it does, why it works, how it is implemented, and every variable in the code.*

---

## The Core Idea — Frequency Domain

Consider a simple 1D signal: an 8-pixel row of brightness values.

```
pixels: [100, 102, 98, 104, 96, 103, 99, 101]
```

These values look similar — they hover around 100 with small variations. In the **spatial domain**, you need all 8 numbers to represent this row. But in the **frequency domain**, this row can be described as:

- A strong "DC component" (the average): ~100
- A weak "1 cycle per block" component: ±3 (the slow variation)
- Essentially zero energy at higher frequencies

You only need 2 numbers to represent what required 8. That's the compression opportunity.

> **Real-World Analogy**: Imagine you are a music teacher listening to a chord. Instead of describing the sound pressure at every millisecond (spatial domain), you describe it as "a C note at 80% volume, an E note at 60% volume, a G note at 40% volume" (frequency domain). The chord description is much more compact than the waveform.
>
> For images: instead of "pixel (0,0)=100, pixel (1,0)=102, ...", you say "this block is mostly 100 (DC), with a gentle leftward brightening (low frequency), and no sharp detail (high frequencies are zero)".

---

## Why DCT Specifically? (Not DFT, Not Wavelet)

The **Discrete Fourier Transform (DFT)** uses complex exponentials (sine + cosine). This creates **Gibbs phenomenon** at block boundaries — ringing artifacts where the block edge appears to the DFT as a sharp discontinuity.

The **Discrete Cosine Transform (DCT)** uses only cosines, and the cosine basis functions have the property of being symmetric — they implicitly assume the signal repeats in a mirror pattern at the block boundaries. This eliminates the edge discontinuity problem and produces much less ringing.

Additionally, the DCT has optimal **energy compaction** for natural images — it concentrates more energy in fewer coefficients than any other orthogonal transform. For the class of signals found in real photographs (smooth gradients, textures, edges), DCT outperforms DFT, Walsh-Hadamard, and wavelet transforms in practice.

---

## The Mathematical Formula — DCT-II

The DCT used in JPEG (and DCTPress) is specifically DCT-II. For an 8-element input vector `x[0..7]`:

```
                    7
X[k] = α(k)/2 · Σ  x[n] · cos((2n+1)·k·π / 16)
                   n=0
```

Where:
- `k` = frequency index (0 to 7); k=0 is DC (lowest frequency), k=7 is highest
- `n` = spatial index (0 to 7); position in the input
- `α(k)` = normalization: `1/√2` for k=0, `1` for k≥1
- `cos(...)` = the cosine basis function for frequency k at position n

**The Inverse DCT-II** (reconstructing pixels from frequencies):

```
              7
x[n] = 1/2 · Σ  α(k) · X[k] · cos((2n+1)·k·π / 16)
             k=0
```

Notice the symmetry: the forward DCT projects data onto cosine basis functions; the inverse reprojects from the basis back to the spatial domain. If no information is lost, the reconstruction is mathematically perfect.

---

## `transform.go` — Line by Line

### The Precomputed Cosine Table

```go
// transform.go
const blockSize = 8

var precomputedCosines [blockSize][blockSize]float64

func init() {
    for u := 0; u < blockSize; u++ {
        for x := 0; x < blockSize; x++ {
            precomputedCosines[u][x] = math.Cos((2*float64(x)+1) * float64(u) * math.Pi / 16.0)
        }
    }
}
```

The variable is named `precomputedCosines[u][x]` where:
- `u` = frequency index (called `k` in the formula above)
- `x` = spatial index (called `n` in the formula above)

The argument to cosine: `(2x+1) · u · π / 16`

Broken down:
- `2x+1`: transforms input index to "half-sample" position (centers between pixels)
- `u`: the frequency we are computing
- `π/16`: scaling factor for 8-element DCT (`π / (2 × 8)`)

**Why precompute?** `math.Cos` calls are expensive (trigonometric function, ~50 ns each). The table has 8×8 = 64 entries. Without precomputation, encoding one 1080p image requires `64 coefficients × (width×height)/64 blocks × 3 channels = 3×1920×1080 ≈ 6 million` cosine calls. With the table, it's 64 table lookups instead of 64 cosine calls per block — roughly a **2.4× speedup**.

### The Alpha Normalization Factor

```go
func alphaFactor(k int) float64 {
    if k == 0 {
        return 1.0 / math.Sqrt2   // = 1/√2 ≈ 0.707
    }
    return 1.0
}
```

The DC coefficient (k=0) uses `1/√2`. All AC coefficients (k≥1) use `1.0`. This normalization ensures the transform is **orthonormal** — the basis vectors have unit length, and `IDCT(DCT(x)) = x` without any scaling.

Without this normalization, the DC coefficient would be √2 times larger than it should be, and the inverse transform would not perfectly reconstruct the original.

### `forwardDCT1D` — One-Dimensional Forward DCT

```go
func forwardDCT1D(x [blockSize]float64) [blockSize]float64 {
    var out [blockSize]float64
    for k := 0; k < blockSize; k++ {       // k = frequency index
        sum := 0.0
        for n := 0; n < blockSize; n++ {   // n = spatial index
            sum += x[n] * precomputedCosines[k][n]
        }
        out[k] = 0.5 * alphaFactor(k) * sum
    }
    return out
}
```

This is a direct implementation of the DCT-II formula:
- Outer loop over `k` (frequency): computes one output coefficient per iteration
- Inner loop over `n` (space): dot product of input with the k-th cosine basis
- `0.5 * alphaFactor(k)` applies the normalization

Complexity: O(N²) for a length-N DCT = O(64) for N=8. For two passes (rows + columns), O(2 × 8 × 64) = O(1024) per block.

### `inverseDCT1D` — One-Dimensional Inverse DCT

```go
func inverseDCT1D(X [blockSize]float64) [blockSize]float64 {
    var out [blockSize]float64
    for n := 0; n < blockSize; n++ {     // n = spatial index (output)
        sum := 0.0
        for k := 0; k < blockSize; k++ { // k = frequency index (input)
            sum += alphaFactor(k) * X[k] * precomputedCosines[k][n]
        }
        out[n] = 0.5 * sum
    }
    return out
}
```

The IDCT is the transpose of the DCT matrix. Same cosine values, same loop structure, but now we are summing over frequency components (k) to reconstruct spatial values (n).

### `ForwardDCT2D` — Separable 2D Transform

The 2D DCT of an 8×8 block is defined as:

```
F(u,v) = α(u)/2 · α(v)/2 · ΣΣ f(x,y) · cos((2x+1)uπ/16) · cos((2y+1)vπ/16)
```

The direct computation requires 8⁴ = 4096 multiplications per block.

**Separability trick**: The 2D formula factors into two independent 1D operations:

```
F(u,v) = [ 1D DCT over columns ] applied to [ 1D DCT over rows ]
```

This reduces complexity from O(N⁴) to O(N³) per block.

```go
func ForwardDCT2D(block [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
    var tmp [blockSize][blockSize]float64
    
    // Pass 1: 1D DCT along each row → tmp[x][v]
    for x := 0; x < blockSize; x++ {
        tmp[x] = forwardDCT1D(block[x])
    }
    
    // Pass 2: 1D DCT along each column of tmp → result[u][v]
    var result [blockSize][blockSize]float64
    for v := 0; v < blockSize; v++ {
        var col [blockSize]float64
        for x := 0; x < blockSize; x++ {
            col[x] = tmp[x][v]  // extract column v from tmp
        }
        dct := forwardDCT1D(col)
        for u := 0; u < blockSize; u++ {
            result[u][v] = dct[u]
        }
    }
    return result
}
```

**Why the transpose step (Pass 2)?** After Pass 1, `tmp[x][v]` holds the row-transformed values. To apply the column DCT, we need column vectors. The inner loop `for x := 0; x < blockSize; x++ { col[x] = tmp[x][v] }` extracts column `v` from `tmp`. This is a **transpose** operation — reading in column-major order from a row-major array.

**Indexing convention**:
- `block[row][col]` = input pixel at position (row, col)
- `tmp[x][v]` = intermediate: row `x` transformed over columns
- `result[u][v]` = final DCT coefficient at frequency (u, v)
  - `u=0, v=0`: DC (mean of the block)
  - `u=7, v=7`: highest spatial frequency

### `InverseDCT2D` — Separable 2D Inverse

```go
func InverseDCT2D(dctBlock [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
    // Pass 1: 1D IDCT along each row (frequency → intermediate)
    var tmp [blockSize][blockSize]float64
    for u := 0; u < blockSize; u++ {
        tmp[u] = inverseDCT1D(dctBlock[u])
    }
    
    // Pass 2: 1D IDCT along each column (intermediate → spatial)
    var result [blockSize][blockSize]float64
    for y := 0; y < blockSize; y++ {
        var col [blockSize]float64
        for u := 0; u < blockSize; u++ {
            col[u] = tmp[u][y]
        }
        idct := inverseDCT1D(col)
        for x := 0; x < blockSize; x++ {
            result[x][y] = idct[x]
        }
    }
    return result
}
```

Exact mirror of `ForwardDCT2D`. Takes frequency coefficients → produces pixel values in [-128, 127].

---

## What the DCT Output Looks Like

For a realistic 8×8 block of a smooth image region (all pixels ≈ 200):

```
Input block (Y, level-shifted by -128):
[  72  70  74  68  76  71  73  69]
[  69  71  73  67  75  70  72  68]
... (similar values throughout)

DCT output:
F(0,0) =  568.5   ← DC: large, represents average brightness
F(0,1) =    2.1   ← low-frequency horizontal variation: small
F(1,0) =    1.8   ← low-frequency vertical variation: small
F(0,2) =   -0.4   ← medium frequency: very small
...
F(7,7) =    0.02  ← highest frequency: nearly zero
```

Almost all energy is in F(0,0). This is **energy compaction** — the defining property that makes DCT useful for compression. After quantization, almost everything except F(0,0) and a few low-frequency coefficients becomes zero. Those zeros are then efficiently coded.

For a high-frequency block (sharp edge, text):

```
Input: alternating dark and bright pixels
DCT output: energy spread across many coefficients, including high frequencies
```

This is why compression is more visible in fine-detail regions — those blocks have energy in many coefficients, all of which get quantized.

---

## Test Coverage — `dct_test.go`

The test file directly validates the mathematical properties of the transform.

### Test 1: Zero Block

```go
func TestForwardDCT2D_ZeroBlock(t *testing.T) {
    var zeroBlock [8][8]float64
    result := dct.ForwardDCT2D(zeroBlock)
    // every output coefficient must be zero
}
```

DCT of all-zeros must be all-zeros. Linearity property.

### Test 2: Round-Trip

```go
func TestForwardInverseDCT_RoundTrip(t *testing.T) {
    // fill block with known values
    dctBlock := dct.ForwardDCT2D(original)
    reconstructed := dct.InverseDCT2D(dctBlock)
    // each pixel must match to within 1e-9
}
```

IDCT(DCT(x)) = x, with floating-point tolerance. This validates the normalization constants are correct.

### Test 3: Constant Block

```go
func TestForwardDCT2D_ConstantBlock(t *testing.T) {
    // all pixels = 100
    result := dct.ForwardDCT2D(constantBlock)
    // result[0][0] != 0  (DC must be non-zero)
    // all other result[u][v] ≈ 0  (no AC energy for constant input)
}
```

A constant block has no spatial variation → zero AC components. All energy in DC. This is the most fundamental DCT property.

### Benchmark

```go
func BenchmarkForwardDCT2D(b *testing.B) {
    // measures ns/op for one 8×8 block
}
```

Run with `go test -bench=BenchmarkForwardDCT2D`. Used to verify the 2.4× speedup from precomputed cosines.

---

## Complexity Analysis

| Approach | Multiplications per block | Time for 1080p (Y only) |
|----------|--------------------------|------------------------|
| Naive 2D (direct formula) | 8⁴ = 4,096 | ~400 ms |
| Separable 2D (two 1D passes) | 2 × 8² = 1,024 | ~100 ms |
| Separable + precomputed cos | 1,024 (no trig) | ~40 ms |
| Butterfly (AAN algorithm) | ~54 | ~8 ms (estimated) |

DCTPress uses the separable + precomputed approach: simple to implement, correct, and fast enough for interactive use.

---

## Key Vocabulary

| Term | Definition |
|------|-----------|
| **Spatial domain** | The normal pixel representation |
| **Frequency domain** | The DCT coefficient representation |
| **DC coefficient** | F(0,0) — the block average; "direct current" analogy from signal processing |
| **AC coefficient** | F(u,v) for u>0 or v>0 — variations around the mean |
| **Energy compaction** | Most energy concentrates in a few low-frequency coefficients |
| **Separability** | 2D DCT = row DCT then column DCT (reduces O(N⁴) to O(N³)) |
| **Orthonormal** | The DCT basis vectors have unit length and are mutually perpendicular |
| **Level shift** | Subtracting 128 before DCT to center values around zero |

---

*Next: [Phase 04 — Quantization & Human Perception](04_quantization_and_human_perception.md)*
