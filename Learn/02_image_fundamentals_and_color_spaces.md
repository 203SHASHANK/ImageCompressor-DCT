# Phase 02 — Image Fundamentals & Color Spaces

> *Before compressing an image, you need to understand what an image actually is, how colors are represented, and why JPEG (and DCTPress) use a different color space than the one your monitor uses.*

---

## What Is a Digital Image?

A digital image is a **rectangular grid of pixels**. Each pixel is a color value. For an 1920×1080 image:

- 1920 pixels wide × 1080 pixels tall = **2,073,600 pixels**
- Each pixel in RGB needs **3 bytes** (Red, Green, Blue, each 0–255)
- Total raw size: 2,073,600 × 3 = **6,220,800 bytes ≈ 6.2 MB**

This is the "original size" DCTPress reports in `CompressionResult.OriginalSize`:

```go
// encoder.go
originalSize := width * height * 3 // RGB bytes
```

A 1080p JPEG on your phone might be 2 MB. The raw data is 6.2 MB. Compression ratio ≈ 3×. DCTPress achieves 48×.

---

## The RGB Color Model

Your monitor has three types of LED subpixels: Red, Green, and Blue. Every visible color is a mixture of these three. A pixel value `(255, 0, 0)` is pure red. `(0, 255, 0)` is pure green. `(128, 128, 128)` is medium gray.

### Why RGB is Not Ideal for Compression

RGB was designed for **display hardware**, not for compression. It has a fundamental problem:

**All three channels carry roughly equal perceptual importance.** If you compress the R channel more than G or B, the image looks wrong — human eyes are equally sensitive to errors in red, green, and blue.

But that's not true of spatial resolution. The human eye has **four times more** luminance (brightness) receptors than color difference receptors in the retina. You can halve the color resolution of an image and most people won't notice.

RGB does not separate brightness from color. If you halve all three channels, you lose both brightness and color detail simultaneously.

---

## The YCbCr Color Model

YCbCr separates a color into:

- **Y** — Luminance (brightness, grayscale value)
- **Cb** — Chrominance Blue (how much the color deviates from gray toward blue)
- **Cr** — Chrominance Red (how much the color deviates from gray toward red)

### The Conversion Formula (ITU-R BT.601)

```go
// encoder.go — extractYCbCrPlanes()
yPlane[row][col]  =  0.299*rf + 0.587*gf + 0.114*bf - 128.0
cbPlane[row][col] = -0.168736*rf - 0.331264*gf + 0.5*bf
crPlane[row][col] =  0.5*rf - 0.418688*gf - 0.081312*bf
```

**Breaking down the Y formula**: `0.299·R + 0.587·G + 0.114·B`

The weights are not equal:
- Green contributes the most (0.587) — human eyes are most sensitive to green wavelengths
- Red contributes moderately (0.299)
- Blue contributes least (0.114) — human eyes are least sensitive to pure blue

These weights match the spectral sensitivity of human photoreceptors, derived from psychophysical experiments.

**The `- 128.0` level shift**: After converting to Y, the range is [0, 255]. The DCT assumes a **zero-mean signal**. Without the shift, a bright block would have a Y value near +200, and the DC coefficient (which represents the block mean) would be enormous, wasting quantization precision. Subtracting 128 centers Y around zero, so a typical block has values in [-128, 127].

**Cb and Cr ranges**: Unlike Y, Cb and Cr are not shifted. They naturally center around zero — a perfectly neutral gray has Cb = 0 and Cr = 0. Blue-dominant pixels have positive Cb; red-dominant pixels have positive Cr.

> **Real-World Analogy**: YCbCr is like separating a photograph into a black-and-white print (Y) and two color tinting layers (Cb, Cr). The black-and-white print carries all the sharpness information. The color tinting can be applied at lower resolution — you've seen this effect in old hand-tinted photographs where the color is slightly blurry but the image still looks natural.

### The Inverse Conversion (YCbCr → RGB)

```go
// encoder.go — ycbcrToRGB()
yVal += 128.0  // undo the level shift
red   = yVal + 1.402*crVal
green = yVal - 0.344136*cbVal - 0.714136*crVal
blue  = yVal + 1.772*cbVal
```

This is the exact inverse of the forward conversion. Notice that reconstructing Red and Blue requires only one chroma channel each, while Green requires both (because Green was most responsible for carrying luminance in the forward direction).

The `clamp` function limits the output to [0, 255] — DCT quantization can push reconstructed values slightly out of range, and the monitor cannot display negative brightness or brightness above 255.

---

## Chroma Subsampling — Exploiting the HVS

**HVS** = Human Visual System.

Since Y (brightness) is far more perceptually important than Cb/Cr (color), we can store Cb and Cr at reduced resolution without visible quality loss. This is called **chroma subsampling**.

### The 4:X:X Notation

The notation `4:Y:Z` describes sampling rates relative to a 4-pixel reference block:
- **4** — Y (luma) always sampled at full resolution
- **Y** — horizontal chroma samples per row
- **Z** — horizontal chroma samples in alternate rows (for vertical subsampling)

| Mode | Cb/Cr resolution | Data saved | When to use |
|------|-----------------|-----------|-------------|
| 4:4:4 | Full (same as Y) | 0% | Medical imaging, professional editing |
| 4:2:2 | Half width, full height | 33% | Video production |
| 4:2:0 | Half width, half height | 50% | JPEG, consumer video, default in DCTPress |

### The Code — `subsampleChroma`

```go
// encoder.go
func (encoder *Encoder) subsampleChroma(cbPlane, crPlane [][]float64, width, height int) ([][]float64, [][]float64) {
    switch encoder.options.ChromaSubsampling {
    case Subsampling420:
        return downsample2D(cbPlane, width, height), downsample2D(crPlane, width, height)
    case Subsampling422:
        return downsampleHorizontal(cbPlane, width, height), downsampleHorizontal(crPlane, width, height)
    default:
        return cbPlane, crPlane  // 4:4:4 — no subsampling
    }
}
```

**`downsample2D` — 4:2:0 subsampling** (averaging 2×2 regions):

```go
func downsample2D(plane [][]float64, width, height int) [][]float64 {
    newHeight := ceilDiv(height, 2)
    newWidth := ceilDiv(width, 2)
    result := make([][]float64, newHeight)
    for row := 0; row < newHeight; row++ {
        result[row] = make([]float64, newWidth)
        for col := 0; col < newWidth; col++ {
            result[row][col] = averageRegion(plane, row*2, col*2, height, width)
        }
    }
    return result
}
```

For a 1920×1080 image:
- Y plane: 1920×1080 (unchanged)
- Cb plane after 4:2:0: 960×540
- Cr plane after 4:2:0: 960×540

Total data before: 1920×1080 × 3 = 6,220,800 values
Total data after: 1920×1080 + 960×540 + 960×540 = 3,110,400 values

4:2:0 subsampling alone halves the data volume before any DCT or quantization.

**Why average, not just pick one pixel?** Simple decimation (just taking every other pixel) causes **aliasing** — sharp color transitions look pixelated. Averaging a 2×2 region is the simplest low-pass filter that prevents aliasing. In production codecs, more sophisticated anti-aliasing filters are used (Lanczos, Kaiser), but 2×2 averaging is correct and simple.

---

## Block Splitting and Padding

After color conversion and subsampling, each channel is divided into 8×8 blocks.

### Why 8×8?

The choice of 8×8 is not arbitrary:

1. **Cache efficiency**: An 8×8 block of float64 is 8 × 8 × 8 = 512 bytes — fits in L1 cache on most CPUs
2. **DCT efficiency**: The 8-point DCT has special fast algorithms (butterfly, AAN) that work specifically on 8 elements
3. **Quantization tables**: The ISO JPEG Annex K tables are calibrated for 8×8 blocks
4. **Human perception**: 8 pixels is about the scale at which spatial frequency sensitivity transitions from "can distinguish" to "cannot distinguish" at typical viewing distances

Larger blocks (16×16, 32×32) exist in newer codecs (H.264, HEVC, AV1) and give better compression, but they also produce more visible **ringing artifacts** at block boundaries when heavily quantized.

### Padding

What if the image width is not a multiple of 8? For example, a 1920×1081 image:

- 1081 is not divisible by 8: 1081 / 8 = 135.125
- We need 136 blocks, which requires 1088 rows
- The extra 7 rows must be filled with something

DCTPress uses **edge replication** (clamping):

```go
// encoder.go — extractBlock()
if srcRow >= planeHeight {
    srcRow = planeHeight - 1  // clamp to last valid row
}
if srcCol >= planeWidth {
    srcCol = planeWidth - 1   // clamp to last valid column
}
```

This repeats the last row/column into the padding region. Alternative approaches:
- **Zero padding**: fills with 0 — creates an artificial edge that generates DCT artifacts
- **Mirror padding**: reflects the image — good for edges but complex
- **Edge replication** (used here): simple, avoids artificial edges, used by JPEG

On decode, the extra padded rows/columns are trimmed:

```go
// decoder.go — decompressChannel()
trimmed := make([][]float64, height)
for row := 0; row < height; row++ {
    trimmed[row] = plane[row][:width]  // slice to original width
}
```

---

## The `image.Image` Interface in Practice

The upload handler receives an image from the browser:

```go
// handlers.go
img, format, err := image.Decode(file)
```

`image.Decode` reads the file header, detects the format (PNG, JPEG, WebP, BMP, TIFF), loads the registered decoder, and returns an `image.Image`. The `format` string tells you what format it was.

The encoder accesses pixels through the interface:

```go
// encoder.go — extractYCbCrPlanes()
r, g, b, _ := img.At(bounds.Min.X+col, bounds.Min.Y+row).RGBA()
rf := float64(r >> 8)
gf := float64(g >> 8)
bf := float64(b >> 8)
```

`RGBA()` returns 16-bit values (0–65535, representing the full premultiplied color). `>> 8` shifts to 8-bit (0–255) by discarding the lower 8 bits. This is the standard Go idiom for converting from `color.Color` to 8-bit values.

Why does `RGBA()` return 16-bit? Because some image formats (PNG, TIFF) support 16-bit color depth. The interface must accommodate the highest precision format, so all implementations return 16-bit.

### Format Registration via Blank Import

```go
// image_loader.go
import (
    _ "image/jpeg"  // register JPEG decoder
    _ "image/png"   // register PNG decoder
    _ "golang.org/x/image/bmp"
    _ "golang.org/x/image/tiff"
    _ "golang.org/x/image/webp"
)
```

Go's `image.Decode` uses a registry of format decoders. Each decoder package registers itself in its `init()` function. The blank import `_` triggers the `init()` without importing any symbols. Without these imports, `image.Decode` would fail for all formats except the ones already registered elsewhere.

---

## Data Flow Summary

```
Browser uploads JPEG file
        │
        ▼
image.Decode()  ──────────────────── detects JPEG, decodes to image.RGBA
        │
        ▼
extractYCbCrPlanes()
    - iterate all pixels via img.At(x, y).RGBA()
    - apply ITU-R BT.601 matrix
    - subtract 128 from Y
    - produce three [][]float64 planes:
        yPlane[height][width]
        cbPlane[height][width]
        crPlane[height][width]
        │
        ▼
subsampleChroma()  (4:2:0 by default)
    - cbPlane: height/2 × width/2
    - crPlane: height/2 × width/2
        │
        ▼
Three independent planes ready for DCT processing
```

---

## Key Numbers to Remember

| Quantity | Value |
|---------|-------|
| Y range after level shift | [-128, 127] |
| Cb/Cr range | [-128, 127] (naturally centered at 0) |
| Y weight for Green | 0.587 (largest — eyes most sensitive to green) |
| Y weight for Blue | 0.114 (smallest — eyes least sensitive to blue) |
| 4:2:0 data reduction | 50% before any DCT |
| Block size | 8×8 pixels |
| Block data | 64 float64 values per channel |

---

*Next: [Phase 03 — The DCT Algorithm](03_dct_algorithm.md)*
