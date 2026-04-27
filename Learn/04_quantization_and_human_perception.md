# Phase 04 — Quantization & Human Perception

> *Quantization is where lossiness happens. Everything else in the pipeline is reversible; quantization is not. Understanding why certain coefficients can be quantized coarsely without visible damage requires understanding how human eyes work.*

---

## What Is Quantization?

Quantization is the process of **rounding a continuous value to the nearest discrete step**.

Simple example:
```
Exact DCT coefficient: 47.83
Step size (quantization table entry): 10
Quantized value: round(47.83 / 10) = round(4.783) = 5
Reconstructed value: 5 × 10 = 50
```

The original was 47.83. The reconstructed is 50. **Error: 2.17.** This error cannot be recovered — it is the "loss" in lossy compression. Smaller step sizes → smaller error → larger file. Larger step sizes → larger error → smaller file.

### The Division and Round

```go
// quantization.go
func quantizeBlock(dctBlock [blockSize][blockSize]float64, table [blockSize][blockSize]float64) [blockSize][blockSize]int {
    var quantized [blockSize][blockSize]int
    for u := 0; u < blockSize; u++ {
        for v := 0; v < blockSize; v++ {
            quantized[u][v] = int(math.Round(dctBlock[u][v] / table[u][v]))
        }
    }
    return quantized
}
```

Each DCT coefficient `dctBlock[u][v]` is divided by the table entry `table[u][v]` and rounded to the nearest integer. The result is a small integer (positive, negative, or zero). For high-frequency coefficients with large step sizes, many coefficients round to zero — and zeros are extremely cheap to store.

### Dequantization — The Inverse

```go
func dequantizeBlock(quantized [blockSize][blockSize]int, table [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
    var dctBlock [blockSize][blockSize]float64
    for u := 0; u < blockSize; u++ {
        for v := 0; v < blockSize; v++ {
            dctBlock[u][v] = float64(quantized[u][v]) * table[u][v]
        }
    }
    return dctBlock
}
```

Multiply the integer back by the step size to get an approximate DCT coefficient. The error from rounding cannot be recovered here — it is permanently baked in.

---

## The Human Visual System and Spatial Frequency Sensitivity

The key insight behind perceptual quantization is:

> **The human eye is not equally sensitive to all spatial frequencies.**

The eye's sensitivity follows a roughly **band-pass shape** with respect to spatial frequency:

- Very low frequencies (slow gradients): high sensitivity
- Medium frequencies (gentle textures, soft edges): highest sensitivity
- High frequencies (sharp edges, fine detail, noise): low sensitivity

This means:
- DCT coefficients representing **low frequencies** (F(0,0), F(0,1), F(1,0), etc.) → **must be quantized finely** (small step size)
- DCT coefficients representing **high frequencies** (F(5,5), F(6,7), F(7,7), etc.) → **can be quantized coarsely** (large step size)

This is exactly what the JPEG quantization tables encode.

> **Real-World Analogy**: Imagine you are writing a description of a landscape painting. You spend many words describing the color of the sky and the position of trees (low frequency — coarse structure). You spend fewer words on the exact texture of the bark on a tree trunk (high frequency — fine detail). The reader can reconstruct a good impression with less detail about the bark.

---

## The ISO Annex K Quantization Tables

These tables were derived from psychophysical experiments — researchers showed human subjects images at various quality levels and recorded what frequency ranges they could distinguish from the original. The result is two 8×8 matrices.

### Luminance Table (Y channel)

```go
// quantization.go
var standardLuminanceTable = [blockSize][blockSize]float64{
    {16, 11, 10, 16, 24, 40, 51, 61},
    {12, 12, 14, 19, 26, 58, 60, 55},
    {14, 13, 16, 24, 40, 57, 69, 56},
    {14, 17, 22, 29, 51, 87, 80, 62},
    {18, 22, 37, 56, 68, 109, 103, 77},
    {24, 35, 55, 64, 81, 104, 113, 92},
    {49, 64, 78, 87, 103, 121, 120, 101},
    {72, 92, 95, 98, 112, 100, 103, 99},
}
```

Reading this table:
- `[0][0] = 16`: DC coefficient step size. This is the most perceptually important — small step size means fine quantization.
- `[0][1] = 11`, `[1][0] = 12`: Adjacent low-frequency coefficients. Even finer than DC in horizontal direction!
- `[7][7] = 99`: Highest spatial frequency, both horizontal and vertical. Step size 99 means this coefficient can be off by up to ±49 without anyone noticing.

The table is **not monotonic** — it doesn't simply increase as you go from top-left to bottom-right. The exact values are empirically derived and somewhat irregular, reflecting the complexity of the human visual system.

### Chrominance Table (Cb/Cr channels)

```go
var standardChrominanceTable = [blockSize][blockSize]float64{
    {17, 18, 24, 47, 99, 99, 99, 99},
    {18, 21, 26, 66, 99, 99, 99, 99},
    {24, 26, 56, 99, 99, 99, 99, 99},
    {47, 66, 99, 99, 99, 99, 99, 99},
    {99, 99, 99, 99, 99, 99, 99, 99},
    ...  // rest are all 99
}
```

Compared to the luminance table:
- Maximum value: 99 (capped by the 1-byte field in JPEG)
- Much of the table is already at maximum (99)
- Only the top-left 3×3 or 4×4 region has meaningful variation

This reflects the much lower sensitivity of the HVS to color detail vs brightness detail. Chroma coefficients at medium-high frequencies can all have step size 99 without any visible degradation.

---

## Quality Scaling — `qualityScaleFactor`

The ISO tables represent "quality 50" — a baseline. The quality slider (1–100) in DCTPress multiplies these base tables by a scale factor.

```go
// quantization.go
func qualityScaleFactor(quality int) float64 {
    if quality <= 0 {
        quality = 1
    }
    if quality > 100 {
        quality = 100
    }
    if quality < 50 {
        return 50.0 / float64(quality)   // quality 1 → scale 50×; quality 25 → scale 2×
    }
    return 2.0 - float64(quality)/50.0   // quality 50 → scale 1×; quality 100 → scale 0
}
```

**The two formulas — why different?**

For quality < 50: scale = `50 / quality`
- quality=1: scale=50 → tables × 50 → huge step sizes → maximum compression, poor quality
- quality=25: scale=2 → tables × 2 → double step sizes → moderate compression
- quality=49: scale≈1.02 → barely above baseline

For quality ≥ 50: scale = `2 - quality/50`
- quality=50: scale=1 → baseline tables (the ISO standard)
- quality=75: scale=0.5 → step sizes × 0.5 → finer quantization → better quality
- quality=100: scale=0 → step sizes × 0 → rounded up to 1 (minimum) → near-lossless

The two formulas meet at quality=50 where both give scale=1. This is the standard JPEG quality formula.

### Building the Scaled Table

```go
func buildQuantizationTable(base [blockSize][blockSize]float64, quality int) [blockSize][blockSize]float64 {
    scale := qualityScaleFactor(quality)
    var table [blockSize][blockSize]float64
    for u := 0; u < blockSize; u++ {
        for v := 0; v < blockSize; v++ {
            value := math.Round(base[u][v] * scale)
            if value < 1 { value = 1 }   // minimum step size = 1 (prevent division by zero)
            if value > 255 { value = 255 } // maximum step size = 255 (1 byte in JPEG format)
            
            // DC coefficient special treatment (see below)
            if u == 0 && v == 0 {
                value = math.Max(1, math.Round(value*0.80))
            }
            table[u][v] = value
        }
    }
    return table
}
```

Clamping to [1, 255]:
- Minimum 1: prevents division by zero in `quantizeBlock` (step size must be at least 1)
- Maximum 255: the JPEG standard stores quantization table entries in 1 byte

**Visual effect of quality settings:**

| Quality | Scale | DC step (luma) | High-freq step (luma) | Result |
|---------|-------|---------------|----------------------|--------|
| 1 | 50× | 640→255 (clamped) | 9999→255 | Heavily blocky |
| 25 | 2× | 26 | 198 | Visible artifacts |
| 50 | 1× | 13 (after DC tune) | 99 | Baseline JPEG |
| 75 | 0.5× | 6 (after DC tune) | 49 | Good quality |
| 95 | 0.1× | 1 | 9 | Near-lossless |
| 100 | 0× | 1 | 1 | Lossless-ish (still rounded) |

---

## The DC Coefficient Tuning (DCTPress-Specific)

```go
// quantization.go
if u == 0 && v == 0 {
    value = math.Max(1, math.Round(value*0.80))  // 20% tighter DC step
}
```

This is a **DCTPress-specific optimization** not present in standard JPEG.

**Why?** The DC coefficient F(0,0) represents the **mean luminance of the block**. If this value is wrong, the entire 8×8 block appears the wrong brightness — even if all the AC coefficients are perfect. It has the highest perceptual impact of any single coefficient.

By reducing the DC step size by 20%, DCTPress gives the DC coefficient finer quantization at a small cost:

- Cost: ~0.4 KB larger file on a typical 1080p image
- Benefit: ~0.08 dB higher PSNR

This is a favorable trade. The benchmark shows DCTPress achieving 4% smaller files than Go stdlib JPEG at the same PSNR — the DC tuning contributes to this.

**Why 0.80 specifically?** From experimentation in the implementation journey. The README notes that "aggressive deadzone quantization" (a different approach) was tried and rejected. The 20% DC tightening is the approach that survived.

---

## What Happens to Zeros?

After quantization, most high-frequency coefficients are zero:

```
DCT block (hypothetical smooth image region):
[568.5,  2.1,  0.3,  0.0,  0.0,  0.0,  0.0,  0.0]
[  1.8,  0.4,  0.1,  0.0, ...]
[  0.2,  0.0, ...]
[  0.0, ...]
...

After quantization with luma table at quality=75:
[ 95,    0,    0,    0,    0,    0,    0,    0]  ← row 0
[  0,    0,    0,    0,    0, ...]               ← all zeros
...
```

The quantized block has mostly zeros. The zigzag ordering (Phase 05) and EOB marker then efficiently represent this sparse structure.

**Calculating the sparsity**: At quality=75, a typical natural image has 5–15 non-zero AC coefficients per block out of 63. That's **76–92% zeros**. Combined with EOB truncation, you only need to store 6–16 values per block instead of 64.

---

## What Didn't Work — Deadzone Quantization

The README documents this failed experiment:

> "Aggressive deadzone quantization — setting a deadzone of 0.64× the step size for high-frequency coefficients (u+v > 8) increased compression by ~2% but dropped PSNR by 0.26 dB. Not worth it."

**What is a deadzone?** Standard quantization rounds to nearest integer: values in (-0.5, +0.5) round to zero. A deadzone expands this: values in (-d, +d) round to zero (for deadzone size d, typically d > 0.5).

The idea: high-frequency coefficients near zero are usually noise, not signal. Forcing them to zero (via a wider deadzone) increases compression. But the PSNR penalty (0.26 dB) was larger than the DC tuning gain (0.08 dB), making it a net loss.

---

## Summary — The Quantization Design Decisions

| Decision | Reason |
|---------|--------|
| Use ISO Annex K base tables | Empirically derived from human perception research |
| Quality 1–100 with JPEG formula | Standard, familiar, tested |
| Clamp table entries to [1, 255] | Prevent division by zero; match JPEG field size |
| Tighten DC step by 20% | Highest PSNR gain per byte of extra size |
| No deadzone for AC coefficients | Tested, made quality worse |
| Separate luma/chroma tables | Eye is more sensitive to luma than chroma |

---

*Next: [Phase 05 — The Full Encode Pipeline](05_encode_pipeline.md)*
