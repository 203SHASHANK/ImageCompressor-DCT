# Phase 08 — Quality Metrics: PSNR and SSIM

> *After compression, how do you know how much quality was lost? Two metrics are used: PSNR (mathematical fidelity) and SSIM (perceptual similarity). Understanding both requires knowing how they differ and what each actually measures.*

---

## Why Measure Quality?

Lossy compression makes a trade-off: smaller file ↔ more distortion. Without a metric, you cannot:
- Compare two codecs at the same "quality"
- Choose a quality setting for a given use case
- Know if an optimization improved or harmed the result

DCTPress measures both PSNR and SSIM after every compression cycle and reports them in the API response. This lets users and developers see exactly what quality was achieved.

---

## Mean Squared Error (MSE) — The Foundation

```go
// metrics/psnr.go
func CalculateMSE(original, compressed image.Image) (float64, error) {
    bounds := original.Bounds()
    sumSquaredError := 0.0
    pixelCount := 0

    for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
        for x := bounds.Min.X; x < bounds.Max.X; x++ {
            r1, g1, b1, _ := original.At(x, y).RGBA()
            r2, g2, b2, _ := compressed.At(x, y).RGBA()

            rf1, gf1, bf1 := float64(r1>>8), float64(g1>>8), float64(b1>>8)
            rf2, gf2, bf2 := float64(r2>>8), float64(g2>>8), float64(b2>>8)

            sumSquaredError += math.Pow(rf1-rf2, 2) + math.Pow(gf1-gf2, 2) + math.Pow(bf1-bf2, 2)
            pixelCount += 3
        }
    }
    return sumSquaredError / float64(pixelCount), nil
}
```

**MSE** = average of squared differences over all pixels and all channels.

Formula: `MSE = (1/N) Σ (original_i - compressed_i)²`

Where N = total number of channel samples = width × height × 3 (R, G, B for every pixel).

**Why squared?** Squaring ensures:
1. Negative differences don't cancel positive ones
2. Large errors are penalized more than small ones (a single 20-unit error counts 4× more than two 10-unit errors)

**Why include all three channels?** Color errors are perceptible just as luminance errors are. A correctly bright image with the wrong color is still wrong.

**`pixelCount += 3`**: Three channel samples per pixel. The MSE is normalized per sample, not per pixel.

---

## PSNR — Peak Signal-to-Noise Ratio

```go
func CalculatePSNR(original, compressed image.Image) (float64, error) {
    mse, err := CalculateMSE(original, compressed)
    if err != nil { ... }
    if mse == 0 {
        return math.Inf(1), nil  // identical images: PSNR = +∞
    }
    const maxPixelValue = 255.0
    psnr := 10 * math.Log10((maxPixelValue * maxPixelValue) / mse)
    return psnr, nil
}
```

**Formula**: `PSNR = 10 · log₁₀(MAX² / MSE)`

Where MAX = 255 (maximum possible pixel value for 8-bit channels).

**Why logarithm?** Human perception of brightness is approximately logarithmic (Weber's law). Doubling the light intensity is perceived as a constant brightness increase, not a doubling. PSNR in decibels (dB) matches this perceptual scale better than raw MSE.

**Why MAX²?** PSNR measures the ratio of the maximum possible signal power (255² = 65025) to the noise power (MSE). Higher ratio = more signal vs noise = better quality.

### PSNR Interpretation

| PSNR | Interpretation |
|------|---------------|
| +∞ | Identical images (MSE = 0) |
| > 40 dB | Excellent — visually indistinguishable from original |
| 35–40 dB | Good — slight quality loss, not noticeable in most contexts |
| 30–35 dB | Acceptable — some visible artifacts on close inspection |
| < 30 dB | Noticeable degradation |

DCTPress achieves 33.77 dB at quality=75 on a 1080p image — solidly in the "acceptable" range, comparable to commercial JPEG.

### PSNR Limitations

PSNR is a mathematical measure, not a perceptual one. Known failures:
- **Blurring**: a blurred image can have high PSNR (small average error) but look terrible
- **Spatial shift**: shifting an image by 1 pixel gives low PSNR but looks identical
- **Contrast enhancement**: enhancing contrast looks better to humans but lowers PSNR

This is why SSIM was developed.

---

## SSIM — Structural Similarity Index

SSIM was proposed by Wang et al. in 2004 as a perceptual quality metric. Instead of measuring pixel-level errors, it compares **structural patterns** between the original and compressed image.

### The Formula

```
SSIM(x, y) = (2·μₓ·μᵧ + C₁)(2·σₓᵧ + C₂)
             ─────────────────────────────────
             (μₓ² + μᵧ² + C₁)(σₓ² + σᵧ² + C₂)
```

Where (computed over a local window):
- `μₓ`, `μᵧ` = mean luminance of original and compressed windows
- `σₓ²`, `σᵧ²` = variance (how much pixels vary around the mean)
- `σₓᵧ` = covariance (how original and compressed vary together)
- `C₁ = (k₁ · L)²`, `C₂ = (k₂ · L)²` = stability constants (k₁=0.01, k₂=0.03, L=255)

The three components capture:
1. **Luminance**: `(2·μₓ·μᵧ + C₁) / (μₓ² + μᵧ² + C₁)` — similarity of brightness
2. **Contrast**: comparison of standard deviations — similarity of local contrast
3. **Structure**: `(2·σₓᵧ + C₂) / (σₓ² + σᵧ² + C₂)` — correlation of patterns

> **Real-World Analogy**: PSNR is like comparing two books character by character and counting mismatches. SSIM is like asking a reader whether both books convey the same meaning — it cares about patterns, not individual characters.

### The DCTPress Implementation

```go
// metrics/ssim.go
func CalculateSSIM(original, compressed image.Image) (float64, error) {
    // Extract luminance planes
    originalLuma := extractLuminance(original)
    compressedLuma := extractLuminance(compressed)

    const k1, k2, dynamicRange = 0.01, 0.03, 255.0
    c1 := (k1 * dynamicRange) * (k1 * dynamicRange)  // = (0.01 × 255)² = 6.5025
    c2 := (k2 * dynamicRange) * (k2 * dynamicRange)  // = (0.03 × 255)² = 58.5225

    ssimSum := 0.0
    windowCount := 0

    for row := 0; row <= height-ssimWindowSize; row += ssimWindowSize/2 {
        for col := 0; col <= width-ssimWindowSize; col += ssimWindowSize/2 {
            meanX, meanY, varX, varY, covarXY := windowStats(
                originalLuma, compressedLuma, row, col, width, height,
            )
            numerator   := (2*meanX*meanY + c1) * (2*covarXY + c2)
            denominator := (meanX*meanX + meanY*meanY + c1) * (varX + varY + c2)
            if denominator != 0 {
                ssimSum += numerator / denominator
                windowCount++
            }
        }
    }
    return ssimSum / float64(windowCount), nil
}
```

**Luminance only**: SSIM is computed on the Y channel (luminance) only, matching human perception (we are more sensitive to brightness structure than color structure).

**`ssimWindowSize = 8`**: Each window is 8×8 pixels. The windows slide with step = `ssimWindowSize/2 = 4` pixels, giving 50% overlap. Dense coverage.

**Why `ssimWindowSize/2` step?** At full step (no overlap), windows at block boundaries might not capture edge artifacts well. 50% overlap gives dense coverage and more stable average.

**Why divide `ssimWindowSize/2` rather than `ssimWindowSize/4`?** Diminishing returns: more overlap → more windows → slower computation, but the SSIM score improvement is marginal beyond 50%.

### `windowStats` — Computing Local Statistics

```go
func windowStats(planeX, planeY [][]float64, startRow, startCol, width, height int) (meanX, meanY, varX, varY, covarXY float64) {
    count := 0.0

    // First pass: compute means
    for row := startRow; row < startRow+ssimWindowSize && row < height; row++ {
        for col := startCol; col < startCol+ssimWindowSize && col < width; col++ {
            meanX += planeX[row][col]
            meanY += planeY[row][col]
            count++
        }
    }
    meanX /= count
    meanY /= count

    // Second pass: compute variance and covariance
    for row := startRow; row < startRow+ssimWindowSize && row < height; row++ {
        for col := startCol; col < startCol+ssimWindowSize && col < width; col++ {
            dx := planeX[row][col] - meanX
            dy := planeY[row][col] - meanY
            varX    += dx * dx
            varY    += dy * dy
            covarXY += dx * dy
        }
    }
    varX    /= count
    varY    /= count
    covarXY /= count
    return
}
```

Two-pass computation:
1. Sum all pixel values, divide by count → means
2. Compute squared deviations from means → variance; cross-product of deviations → covariance

**Named return values**: Go supports naming return values (`(meanX, meanY, varX, varY, covarXY float64)`). A bare `return` at the end returns whatever these variables currently hold. This is useful for multi-value returns where variables are accumulated throughout the function.

**Covariance**: `covarXY = E[(x-μₓ)(y-μᵧ)]` measures how x and y vary together:
- `covarXY > 0`: when x is bright, y is also bright (positive correlation)
- `covarXY ≈ σₓ·σᵧ`: perfect positive correlation (identical structure)
- `covarXY ≈ 0`: no correlation (completely different structure)

### Stability Constants C₁ and C₂

`C₁ = (0.01 × 255)² = 6.5025` and `C₂ = (0.03 × 255)² = 58.5225`.

These prevent division by zero when means or variances are near zero (e.g., a uniform black window). They also stabilize the ratio when values are small. The specific values (k₁=0.01, k₂=0.03) are the standard values from Wang et al. 2004, chosen to give stable results across a wide range of image content.

### SSIM Range and Interpretation

SSIM ranges from [0, 1] (can be negative for completely inverted images, but practically between 0 and 1).

| SSIM | Interpretation |
|------|---------------|
| 1.0 | Identical |
| > 0.95 | Excellent — minimal perceptual difference |
| 0.90–0.95 | Good — minor artifacts |
| 0.80–0.90 | Acceptable — visible but tolerable |
| < 0.80 | Poor quality |

DCTPress achieves SSIM = 0.9924 at quality=75 — excellent.

### `extractLuminance`

```go
func extractLuminance(img image.Image) [][]float64 {
    bounds := img.Bounds()
    luma := make([][]float64, height)
    for row := 0; row < height; row++ {
        luma[row] = make([]float64, width)
        for col := 0; col < width; col++ {
            r, g, b, _ := img.At(...).RGBA()
            rf := float64(r >> 8)
            gf := float64(g >> 8)
            bf := float64(b >> 8)
            luma[row][col] = 0.299*rf + 0.587*gf + 0.114*bf
        }
    }
    return luma
}
```

Identical coefficients to the YCbCr conversion in the encoder, but without the -128 level shift. SSIM expects luminance in [0, 255], not the centered [-128, 127] used by the DCT.

---

## Why Both Metrics?

| Metric | Strength | Weakness |
|--------|---------|---------|
| PSNR | Fast, simple, widely used | Doesn't correlate well with perception |
| SSIM | Correlates with human perception | Slower (window operations), less interpretable |

Using both gives a complete picture:
- High PSNR + high SSIM = genuinely good quality
- High PSNR + low SSIM = mathematically similar but structurally different (e.g., blurring)
- Low PSNR + high SSIM = rare; would mean large average errors but consistent structure

For DCTPress at quality=75:
- PSNR = 33.77 dB (acceptable range, comparable to commercial JPEG)
- SSIM = 0.9924 (excellent — structural patterns very well preserved)

This tells you: the codec introduces some numeric error (PSNR), but the perceptual structure is nearly perfectly preserved (SSIM).

---

## Integration with the Pipeline

After encoding, the compression service immediately decodes and measures:

```go
// compression_service.go
result, err := encoder.Encode(request.Image)
// ...
decodedImage, err := decoder.Decode(result.Data)
// ...
psnrValue, err := metrics.CalculatePSNR(request.Image, decodedImage)
ssimValue, err := metrics.CalculateSSIM(request.Image, decodedImage)

return &CompressResponse{
    PSNR: psnrValue,
    SSIM: ssimValue,
    ...
}
```

This decode-to-measure pattern is standard in codec evaluation. You always measure the round-trip quality (encode → decode → compare to original), not just the encoder's output.

---

## Test Cases — `metrics_test.go`

```go
func TestCalculatePSNR_IdenticalImages(t *testing.T) {
    // PSNR of identical images must be +Inf
    psnr, _ := metrics.CalculatePSNR(img, img)
    if !math.IsInf(psnr, 1) { ... }
}

func TestCalculatePSNR_DifferentImages(t *testing.T) {
    // Small difference (5 units per pixel) → PSNR > 30 dB
    original := createUniformImage(64, 64, color.RGBA{R: 255, ...})
    compressed := createUniformImage(64, 64, color.RGBA{R: 250, ...})
    psnr, _ := metrics.CalculatePSNR(original, compressed)
    // expected: PSNR > 30
}

func TestCalculateSSIM_IdenticalImages(t *testing.T) {
    // SSIM of identical images must be ~1.0
    ssim, _ := metrics.CalculateSSIM(img, img)
    if math.Abs(ssim - 1.0) > 0.01 { ... }
}

func TestCalculateSSIM_TooSmall(t *testing.T) {
    // 4×4 image → error (smaller than 8×8 window)
}
```

The tests verify the edge cases: identical images, small differences, dimension mismatches, and images too small for SSIM's sliding window.

---

*Next: [Phase 09 — HTTP API & Service Layer](09_http_api_and_services.md)*
