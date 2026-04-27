# Phase 06 — The Full Decode Pipeline

> *Decoding is the exact inverse of encoding. This phase covers `decoder.go` — how the binary `.dct` bytes are turned back into pixels, and why bilinear chroma upsampling gives better quality than nearest-neighbor.*

---

## The Decoder Struct

```go
// decoder.go
type Decoder struct{}

func NewDecoder() *Decoder {
    return &Decoder{}
}
```

The decoder has no configuration. All parameters needed for decoding (width, height, quality, subsampling mode) are stored in the file header. The decoder reads them from there.

---

## `Decode` — Top Level

```go
func (decoder *Decoder) Decode(data []byte) (image.Image, error) {
    // Step 1: Parse header, Huffman-decode channels
    width, height, quality, subsampling, yData, cbData, crData, err := unpackChannels(data)
    if err != nil {
        return nil, fmt.Errorf("failed to unpack compressed data: %w", err)
    }

    // Step 2: Rebuild quantization tables from stored quality setting
    luminanceTable    := buildQuantizationTable(standardLuminanceTable, quality)
    chrominanceTable  := buildQuantizationTable(standardChrominanceTable, quality)

    // Step 3: Decompress each channel
    yPlane := decoder.decompressChannel(yData, width, height, luminanceTable)
    
    cbWidth, cbHeight := chromaDimensions(width, height, ChromaSubsampling(subsampling))
    cbPlane := decoder.decompressChannel(cbData, cbWidth, cbHeight, chrominanceTable)
    crPlane := decoder.decompressChannel(crData, cbWidth, cbHeight, chrominanceTable)

    // Step 4: Upsample chroma if subsampled
    cbPlane = upsampleChroma(cbPlane, width, height, ChromaSubsampling(subsampling))
    crPlane = upsampleChroma(crPlane, width, height, ChromaSubsampling(subsampling))

    // Step 5: Reconstruct RGB image
    return buildRGBAImage(yPlane, cbPlane, crPlane, width, height), nil
}
```

**Key insight**: The decoder rebuilds the quantization tables from the `quality` value stored in the header. It does not store the tables themselves. Since the table construction (`buildQuantizationTable`) is deterministic given the same quality input, this recovers the exact tables used during encoding. This saves storing 2 × 64 × 1 = 128 bytes of table data per file.

**`chromaDimensions`**: Determines the size of the Cb/Cr planes based on the subsampling mode:

```go
func chromaDimensions(width, height int, subsampling ChromaSubsampling) (int, int) {
    switch subsampling {
    case Subsampling420:
        return ceilDiv(width, 2), ceilDiv(height, 2)  // half width, half height
    case Subsampling422:
        return ceilDiv(width, 2), height              // half width, full height
    default:
        return width, height                          // full resolution
    }
}
```

The chroma plane dimensions must match exactly what the encoder produced. If the encoder halved the dimensions, the decoder starts with those halved dimensions and upsamples afterward.

---

## `decompressChannel` — Inverse of `compressChannel`

```go
func (decoder *Decoder) decompressChannel(coefficients []int, width, height int, quantTable [blockSize][blockSize]float64) [][]float64 {
    paddedHeight := ceilDiv(height, blockSize) * blockSize
    paddedWidth := ceilDiv(width, blockSize) * blockSize

    plane := make([][]float64, paddedHeight)
    for row := range plane {
        plane[row] = make([]float64, paddedWidth)  // all initialized to 0.0
    }

    blocksPerRow := paddedWidth / blockSize
    totalBlocks := (paddedHeight / blockSize) * blocksPerRow

    pos := 0     // current position in coefficients slice
    prevDC := 0  // for DC delta decoding

    for blockIndex := 0; blockIndex < totalBlocks && pos < len(coefficients); blockIndex++ {
        var zigzag [64]int

        // Decode DC: add delta to previous DC
        dc := prevDC + coefficients[pos]
        pos++
        prevDC = dc
        zigzag[0] = dc

        // Decode AC values until EOB marker
        for acIdx := 1; pos < len(coefficients); {
            v := coefficients[pos]
            pos++
            if v == eobMarker {
                break  // remaining positions in zigzag are zero (already initialized)
            }
            if acIdx < 64 {
                zigzag[acIdx] = v
                acIdx++
            }
        }

        // Reconstruct block
        quantized   := zigzagUnflatten(zigzag[:])
        dctBlock    := dequantizeBlock(quantized, quantTable)
        spatialBlock := InverseDCT2D(dctBlock)

        // Place block into the padded plane
        blockRow := (blockIndex / blocksPerRow) * blockSize
        blockCol := (blockIndex % blocksPerRow) * blockSize
        for row := 0; row < blockSize; row++ {
            for col := 0; col < blockSize; col++ {
                if blockRow+row < paddedHeight && blockCol+col < paddedWidth {
                    plane[blockRow+row][blockCol+col] = spatialBlock[row][col]
                }
            }
        }
    }

    // Trim padding
    trimmed := make([][]float64, height)
    for row := 0; row < height; row++ {
        trimmed[row] = plane[row][:width]
    }
    return trimmed
}
```

### DC Delta Reconstruction

```go
dc := prevDC + coefficients[pos]
pos++
prevDC = dc
zigzag[0] = dc
```

The encoder stored `dcDelta = dc - prevDC`. The decoder recovers `dc = prevDC + dcDelta`. This is the inverse of delta coding — a cumulative sum running through all blocks in order. The starting value (`prevDC = 0`) must match between encoder and decoder; both start at 0 and evolve identically.

### EOB Handling

```go
for acIdx := 1; pos < len(coefficients); {
    v := coefficients[pos]
    pos++
    if v == eobMarker {
        break
    }
    if acIdx < 64 {
        zigzag[acIdx] = v
        acIdx++
    }
}
```

The `zigzag` array was initialized with zeros (Go's zero-value for integers). When `eobMarker` is encountered, the loop breaks. All `zigzag[acIdx]` through `zigzag[63]` remain zero — exactly what the encoder intended.

`if acIdx < 64` — bounds check prevents a malformed coefficient stream from writing out of bounds. A well-formed file never hits this case (the encoder always writes exactly 63 AC positions maximum).

### Block Reconstruction

```go
quantized    := zigzagUnflatten(zigzag[:])    // 1D → 2D block (integer coefficients)
dctBlock     := dequantizeBlock(quantized, quantTable)  // integer → float (approx DCT coefficients)
spatialBlock := InverseDCT2D(dctBlock)                  // frequencies → pixel values
```

`zigzag[:]` converts the fixed `[64]int` array to a slice (required by `zigzagUnflatten` which takes `[]int`).

### Trimming Padding

```go
trimmed := make([][]float64, height)
for row := 0; row < height; row++ {
    trimmed[row] = plane[row][:width]  // slice to original width (cheap: no copy)
}
```

`plane[row][:width]` creates a new slice header pointing to the same underlying array, but with length `width`. This is a **zero-copy trim** — no data is copied, just the slice header is updated. Extra columns beyond `width` are no longer accessible through `trimmed`.

---

## `buildRGBAImage` — Assembling the Final Image

```go
func buildRGBAImage(yPlane, cbPlane, crPlane [][]float64, width, height int) *image.RGBA {
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    for row := 0; row < height; row++ {
        for col := 0; col < width; col++ {
            pixel := ycbcrToRGB(yPlane[row][col], cbPlane[row][col], crPlane[row][col])
            img.SetRGBA(col, row, pixel)
        }
    }
    return img
}
```

`image.NewRGBA` allocates a flat byte slice of `width × height × 4` bytes (RGBA, 4 bytes per pixel). `SetRGBA` writes 4 bytes at offset `(row*stride + col*4)`.

The `ycbcrToRGB` function (in `encoder.go`) applies the inverse color transform and clamps to [0, 255].

---

## `unpackChannels` — Parsing the Binary Header

```go
func unpackChannels(data []byte) (width, height, quality, subsampling int, yData, cbData, crData []int, err error) {
    const headerSize = 4 + 1 + 4 + 4 + 1 + 1 + 4 + 4 + 4  // 27 bytes

    if len(data) < headerSize {
        return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("data too short for header: %d bytes", len(data))
    }

    if string(data[0:3]) != "DCT" {
        return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("invalid magic bytes")
    }

    width      = int(binary.LittleEndian.Uint32(data[5:9]))
    height     = int(binary.LittleEndian.Uint32(data[9:13]))
    quality    = int(data[13])
    subsampling = int(data[14])
    yLen       = int(binary.LittleEndian.Uint32(data[15:19]))
    cbLen      = int(binary.LittleEndian.Uint32(data[19:23]))
    crLen      = int(binary.LittleEndian.Uint32(data[23:27]))
```

`binary.LittleEndian.Uint32(data[5:9])` reads 4 bytes starting at offset 5 and interprets them as a little-endian uint32. This is the mirror of `appendUint32LE` in the encoder.

```go
    offset := headerSize
    yData, err = huffman.Decode(data[offset : offset+yLen])
    offset += yLen
    cbData, err = huffman.Decode(data[offset : offset+cbLen])
    offset += cbLen
    crData, err = huffman.Decode(data[offset : offset+crLen])
```

`data[offset : offset+yLen]` is a **slice of the original byte slice** — no copy. The Huffman decoder receives a view into the original data buffer and reads from it.

---

## Bilinear Chroma Upsampling — `upsampleChroma`

This is one of the most technically interesting functions in the codebase. After decoding the subsampled chroma planes (e.g., 960×540 for a 1920×1080 image), we need to expand them back to full luma resolution (1920×1080).

The naive approach (nearest-neighbor) would just copy each chroma sample to a 2×2 block of pixels. This creates visible color banding at edges.

DCTPress uses **bilinear interpolation with JPEG cosited siting**.

### JPEG Cosited Siting

"Cosited" means the chroma samples are co-located with the luma samples at even pixel positions. The pixel grid looks like:

```
Luma  pixels:  L  L  L  L  L  L  ...  (every position)
Chroma pixels: C     C     C     ...  (every other position)
```

The chroma sample C at position 0 corresponds to luma pixels 0 and 1. The question is: how do we interpolate the chroma value for pixel position 1?

JPEG cosited siting says: pixel i maps to source coordinate `(i + 0.5) / scale - 0.5`.

### The Bilinear Formula

```go
func upsampleChroma(plane [][]float64, targetWidth, targetHeight int, subsampling ChromaSubsampling) [][]float64 {
    // ...
    bilinear := func(row, col int, scaleY, scaleX float64) float64 {
        // Map target pixel centre to source coordinate (JPEG chroma siting)
        srcY := (float64(row)+0.5)/scaleY - 0.5
        srcX := (float64(col)+0.5)/scaleX - 0.5

        y0 := int(math.Floor(srcY))
        x0 := int(math.Floor(srcX))
        y1 := y0 + 1
        x1 := x0 + 1

        y0 = clampI(y0, 0, srcH-1)
        y1 = clampI(y1, 0, srcH-1)
        x0 = clampI(x0, 0, srcW-1)
        x1 = clampI(x1, 0, srcW-1)

        fy := srcY - math.Floor(srcY)   // fractional part: 0.0 to 1.0
        fx := srcX - math.Floor(srcX)

        // Clamp fractional parts for out-of-bounds source coordinates
        if srcY < 0 { fy = 0 }
        if srcX < 0 { fx = 0 }

        tl := plane[y0][x0]  // top-left
        tr := plane[y0][x1]  // top-right
        bl := plane[y1][x0]  // bottom-left
        br := plane[y1][x1]  // bottom-right

        // Four-sample bilinear blend
        return tl*(1-fx)*(1-fy) + tr*fx*(1-fy) + bl*(1-fx)*fy + br*fx*fy
    }
```

**Breaking down the formula**:

- `srcY = (row + 0.5) / scale - 0.5`: maps target pixel center to source coordinate
  - `+0.5`: shifts from pixel corner to pixel center
  - `/ scale`: scales from target to source resolution  
  - `-0.5`: shifts back from center to corner convention
  - For scale=2 (4:2:0 vertical): row=0 → srcY=-0.25; row=1 → srcY=0.25; row=2 → srcY=0.75; row=3 → srcY=1.25
  
- `y0 = floor(srcY)`, `y1 = y0 + 1`: the two nearest source rows

- `fy = srcY - floor(srcY)`: fractional distance from y0 to srcY (0.0 to 1.0)
  - fy=0.0 → interpolated value equals top row exactly
  - fy=1.0 → interpolated value equals bottom row exactly
  - fy=0.5 → equal blend

- Bilinear blend: `tl*(1-fx)*(1-fy) + tr*fx*(1-fy) + bl*(1-fx)*fy + br*fx*fy`
  - This is the weighted average of 4 surrounding source pixels
  - Weights sum to 1: `(1-fx)(1-fy) + fx(1-fy) + (1-fx)fy + fx·fy = 1`

> **Real-World Analogy**: Bilinear interpolation is like mixing four cans of paint. If you want a color that lies between four paint samples arranged in a square, you mix them proportionally based on how close you are to each corner. Standing at the center → equal blend. Standing near the top-right corner → mostly the top-right color.

### Why Bilinear Beats Nearest-Neighbor

Nearest-neighbor gives each downsampled chroma pixel a 2×2 block of identical color. At color boundaries, this creates visible step changes (banding). Bilinear interpolation smoothly transitions between samples.

The README quantifies this: **~0.15–0.3 dB better PSNR** with no change in compressed file size. This free quality improvement is why DCTPress achieves comparable or better PSNR than Go stdlib JPEG (which uses nearest-neighbor chroma upsampling).

### Edge Clamping for Upsampling

```go
y0 = clampI(y0, 0, srcH-1)
y1 = clampI(y1, 0, srcH-1)
x0 = clampI(x0, 0, srcW-1)
x1 = clampI(x1, 0, srcW-1)
```

For pixels at the image boundary, `srcY` can be negative or beyond `srcH-1`. Clamping reuses the nearest valid source pixel. Combined with `if srcY < 0 { fy = 0 }`, this ensures boundary pixels get the value of the nearest valid source sample without extrapolation.

---

## `upsampleChroma` Call Sites

```go
// decoder.go
switch subsampling {
case Subsampling420:
    result[row][col] = bilinear(row, col, 2.0, 2.0)  // scale 2× in both dimensions
case Subsampling422:
    result[row][col] = bilinear(row, col, 1.0, 2.0)  // scale 2× horizontally only
}
```

For 4:2:0: upsampling by 2× in both directions → every source pixel generates a 2×2 region of target pixels.
For 4:2:2: upsampling by 2× horizontally only → every source pixel generates 2 horizontal target pixels.
For 4:4:4: no upsampling needed, function returns the plane unchanged.

---

## Full Decode Flow

```
Input: []byte (27-byte header + Y/Cb/Cr Huffman blobs)

1. unpackChannels():
   - validate magic bytes "DCT"
   - read width, height, quality, subsampling from header
   - Huffman-decode Y blob → []int (coefficient stream)
   - Huffman-decode Cb blob → []int
   - Huffman-decode Cr blob → []int

2. buildQuantizationTable(standardLuminanceTable, quality) → luminanceTable
   buildQuantizationTable(standardChrominanceTable, quality) → chrominanceTable

3. decompressChannel(yData, width, height, luminanceTable):
   for each block:
     - DC reconstruction: dc = prevDC + delta
     - AC collection until eobMarker
     - zigzagUnflatten() → [8][8]int
     - dequantizeBlock() → [8][8]float64
     - InverseDCT2D() → [8][8]float64
     - place into padded plane
   trim padding → [][]float64 (height × width)

4. Same for Cb, Cr at reduced dimensions

5. upsampleChroma():
   - bilinear interpolation back to full luma resolution
   - for each target pixel: 4-sample weighted blend

6. buildRGBAImage():
   - for each pixel: ycbcrToRGB(y, cb, cr)
   - clamp to [0, 255]
   - write to *image.RGBA

Output: image.Image (the reconstructed RGB image)
```

---

## Decode vs Encode: Symmetry and Asymmetry

| Step | Encode | Decode |
|------|--------|--------|
| Color space | RGB → YCbCr | YCbCr → RGB |
| Chroma | Downsample | Upsample (bilinear) |
| Blocks | Extract + pad | Reconstruct + trim |
| DCT | Forward DCT | Inverse DCT |
| Quantization | divide + round (lossy) | multiply (no loss) |
| Zigzag | flatten | unflatten |
| DC | delta encode | cumulative sum |
| EOB | find last nonzero, write marker | stop on marker |
| Huffman | count → build tree → encode | parse tree → decode |
| Binary | write header + blobs | read header + blobs |

The one **asymmetric** step is quantization: the encoder rounds (lossy); the decoder just multiplies. This is by design — the "loss" happened during encoding and cannot be recovered on decode.

---

*Next: [Phase 07 — Huffman Entropy Coding](07_huffman_entropy_coding.md)*
