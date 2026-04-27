# Phase 05 — The Full Encode Pipeline

> *This phase walks through `encoder.go` from top to bottom, connecting every function to its role in the compression chain.*

---

## The Encoder Struct

```go
// encoder.go
type Encoder struct {
    options               CompressionOptions
    luminanceQuantTable   [blockSize][blockSize]float64
    chrominanceQuantTable [blockSize][blockSize]float64
}

type CompressionOptions struct {
    Quality           int
    ChromaSubsampling ChromaSubsampling
}
```

The `Encoder` is constructed once per compression request and holds:
- `options`: the user-chosen parameters
- `luminanceQuantTable`: the scaled 8×8 step-size table for the Y channel
- `chrominanceQuantTable`: the scaled 8×8 step-size table for Cb/Cr channels

Tables are computed at construction time (not per-block), so every block shares the same table for the session.

### Constructor

```go
func NewEncoder(options CompressionOptions) (*Encoder, error) {
    if options.Quality < 1 || options.Quality > 100 {
        return nil, fmt.Errorf("quality must be between 1 and 100, got %d", options.Quality)
    }
    return &Encoder{
        options:               options,
        luminanceQuantTable:   buildQuantizationTable(standardLuminanceTable, options.Quality),
        chrominanceQuantTable: buildQuantizationTable(standardChrominanceTable, options.Quality),
    }, nil
}
```

**Input validation at the boundary**: Quality is checked here. If invalid, the error propagates up to the HTTP handler, which returns a 400 Bad Request. The core algorithm never sees invalid quality values.

---

## The Encode Function — Top Level

```go
func (encoder *Encoder) Encode(img image.Image) (*models.CompressionResult, error) {
    bounds := img.Bounds()
    width := bounds.Dx()
    height := bounds.Dy()

    // Step 1: Color space conversion
    yPlane, cbPlane, crPlane := extractYCbCrPlanes(img)

    // Step 2: Chroma subsampling
    cbPlane, crPlane = encoder.subsampleChroma(cbPlane, crPlane, width, height)

    // Step 3: Compress each channel independently
    yData  := encoder.compressChannel(yPlane,  encoder.luminanceQuantTable)
    cbData := encoder.compressChannel(cbPlane, encoder.chrominanceQuantTable)
    crData := encoder.compressChannel(crPlane, encoder.chrominanceQuantTable)

    // Step 4: Pack into binary format
    packed := packChannels(width, height, encoder.options.Quality,
        int(encoder.options.ChromaSubsampling), yData, cbData, crData)

    originalSize := width * height * 3
    return &models.CompressionResult{
        Data:             packed,
        OriginalSize:     originalSize,
        CompressedSize:   len(packed),
        CompressionRatio: float64(originalSize) / float64(len(packed)),
        Width:            width,
        Height:           height,
        Quality:          encoder.options.Quality,
    }, nil
}
```

The three channels are encoded independently. This matters:
- Y gets the luminance quantization table (finer, smaller step sizes)
- Cb and Cr get the chrominance table (coarser, larger step sizes)
- The Huffman tree is built separately per channel (adapts to each channel's statistics)

`originalSize = width * height * 3` is the baseline: raw uncompressed RGB. The `CompressionRatio` tells you how many times smaller the `.dct` file is compared to raw RGB.

---

## `compressChannel` — The Core of the Encoder

This function compresses one channel (Y, Cb, or Cr) from a 2D float64 plane to a compact integer slice. It has four sub-phases, and the parallelism is the most interesting part.

```go
func (encoder *Encoder) compressChannel(plane [][]float64, quantTable [blockSize][blockSize]float64) []int {
    height := len(plane)
    width := len(plane[0])

    paddedHeight := ceilDiv(height, blockSize) * blockSize
    paddedWidth := ceilDiv(width, blockSize) * blockSize
    numBlockCols := paddedWidth / blockSize
    totalBlocks := (paddedHeight / blockSize) * numBlockCols
```

**Padding calculation**: If height=1080 and blockSize=8, `ceilDiv(1080, 8) * 8 = 1080` (no padding needed, divisible). For height=1081: `ceilDiv(1081, 8) * 8 = 136 * 8 = 1088` (7 rows of padding). `totalBlocks` is the total number of 8×8 blocks across the entire padded plane.

### Parallel Block Processing (Worker Pool)

```go
    type blockWork struct{ idx, row, col int }
    type blockResult struct{ zigzag [64]int }

    processed := make([]blockResult, totalBlocks)  // pre-allocate results slice

    numWorkers := runtime.NumCPU()
    if numWorkers > totalBlocks {
        numWorkers = totalBlocks
    }

    workChan := make(chan blockWork, totalBlocks)  // buffered channel
    var wg sync.WaitGroup

    for w := 0; w < numWorkers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for work := range workChan {
                block := extractBlock(plane, work.row, work.col, height, width)
                dctBlock := ForwardDCT2D(block)
                quantized := quantizeBlock(dctBlock, quantTable)
                processed[work.idx].zigzag = zigzagFlattenFixed(quantized)
            }
        }()
    }

    for idx := 0; idx < totalBlocks; idx++ {
        workChan <- blockWork{
            idx: idx,
            row: (idx / numBlockCols) * blockSize,
            col: (idx % numBlockCols) * blockSize,
        }
    }
    close(workChan)
    wg.Wait()
```

**Why parallel?** DCT + quantize + zigzag for each 8×8 block are **completely independent** — block (3,5) does not depend on block (3,4). This is "embarrassingly parallel" work. On an 8-core machine, this runs ~8× faster than serial.

**The design pattern — Pre-sized results + indexed writes**:

- `processed := make([]blockResult, totalBlocks)` — allocate result space for all blocks upfront
- Each goroutine writes `processed[work.idx]` with the block index
- Multiple goroutines writing **different indices** of a slice is safe without a mutex in Go (different memory locations)

**Why `blockResult{ zigzag [64]int }` as a fixed array?** `zigzagFlattenFixed` returns `[64]int` (a fixed array, not a slice). This avoids a heap allocation per block in the parallel hot path — a fixed array embedded in a struct can be stored directly in the pre-allocated `processed` slice, no GC pressure.

**Why a buffered channel with capacity = totalBlocks?** A buffered channel lets the main goroutine enqueue all blocks without blocking. If the channel were unbuffered, the main goroutine would block after sending each item until a worker consumed it. The full-capacity buffer lets all work items be enqueued in one burst, maximizing parallelism.

**`runtime.NumCPU()`** — returns the number of logical CPU cores. This is the optimal number of workers for CPU-bound tasks.

**`close(workChan)`** — closing a channel signals workers that no more items will be sent. Workers' `for work := range workChan` loops terminate when the channel is closed and drained.

**`wg.Wait()`** — blocks until all workers have finished and called `wg.Done()`.

> **Real-World Analogy**: The worker pool is like a pizza restaurant with multiple ovens. The "main goroutine" is the order-taker who writes all orders on slips and puts them in a queue. Each "worker goroutine" is an oven operator who picks up slips and bakes pizzas. All pizzas can be baked simultaneously. The order-taker doesn't wait for each pizza — they just fill the queue.

### Serial DC Delta Coding and EOB Truncation

```go
    // Serial DC delta-encoding and packing (order must be preserved)
    coefficients := make([]int, 0, totalBlocks*10)
    prevDC := 0

    for i := 0; i < totalBlocks; i++ {
        zz := processed[i].zigzag
        dcDelta := zz[0] - prevDC    // DC difference from previous block
        prevDC = zz[0]               // update for next iteration

        // Find index of last non-zero AC coefficient
        lastNonZero := 0
        for k := 63; k >= 1; k-- {
            if zz[k] != 0 {
                lastNonZero = k
                break
            }
        }

        coefficients = append(coefficients, dcDelta)
        for k := 1; k <= lastNonZero; k++ {
            coefficients = append(coefficients, zz[k])
        }
        coefficients = append(coefficients, eobMarker)  // = -32768
    }

    return coefficients
```

**Why serial?** DC delta coding requires the DC value of the previous block. Block N's delta = Block N's DC - Block (N-1)'s DC. This is a sequential dependency — cannot be parallelized.

**DC Delta Coding**: Adjacent blocks in natural images tend to have similar mean brightness. A forest scene might have Y DC values of [95, 93, 94, 91, 95, ...] — the deltas [−2, +1, −3, +4, ...] are small numbers that cluster near zero. Small numbers cluster near zero → Huffman coding assigns them very short codes → more compression.

**EOB Marker (End of Block)**: `eobMarker = -32768 = math.MinInt16`. After the last non-zero AC coefficient, this marker is written. All trailing zeros are **implicit** — the decoder will fill them in. Why -32768 as the sentinel? Because no valid quantized DCT coefficient can have this value (quantized values fit in int16, but -32768 is the most negative int16 value and cannot result from dividing a typical DCT coefficient by a quantization table entry of at least 1).

**Output format per block**:
```
[dcDelta] [ac_1] [ac_2] ... [ac_k] [eobMarker=-32768]
```

For a typical smooth block at quality=75, only dc_delta and 2–3 AC values survive before EOB. Instead of 64 values, you store 4–5.

**`make([]int, 0, totalBlocks*10)`**: Pre-allocate capacity for ~10 values per block (empirical estimate). Pre-allocation avoids repeated `append` reallocations as the slice grows.

---

## Zigzag Ordering — `zigzagOrder` and Related Functions

```go
var zigzagOrder = [64][2]int{
    {0, 0}, {0, 1}, {1, 0}, {2, 0}, {1, 1}, {0, 2}, {0, 3}, {1, 2},
    {2, 1}, {3, 0}, {4, 0}, {3, 1}, {2, 2}, {1, 3}, {0, 4}, {0, 5},
    ...
    {7, 6}, {7, 7},
}
```

This is a hard-coded lookup table defining the zigzag traversal order of an 8×8 block. Position 0 in the sequence is always (0,0) (DC). The sequence traverses diagonals from top-left to bottom-right:

```
DC  → (0,1) → (1,0) → (2,0) → (1,1) → (0,2) → (0,3) → ...
```

Visualized:
```
  [DC ]  [AC1 ]  [AC5 ]  [AC6 ]  [AC14]  [AC15]  [AC27]  [AC28]
  [AC2 ]  [AC4 ]  [AC7 ]  [AC13]  [AC16]  [AC26]  [AC29]  [AC42]
  [AC3 ]  [AC8 ]  [AC12]  [AC17]  [AC25]  [AC30]  [AC41]  [AC43]
  ...
  [AC35]  [AC46]  [AC55]  [AC60]  [AC61]  [AC62]  [AC63]  [AC63]
```

The traversal puts **low-frequency coefficients first** and **high-frequency coefficients last**. In a typical block, the high-frequency end of the zigzag sequence is mostly or entirely zeros. EOB truncation then drops all trailing zeros with a single marker.

**Why this specific diagonal traversal?** The 2D spatial frequency increases as you move diagonally (increasing u+v). The exact diagonal order is designed to maximize the run of zeros at the end of the sequence.

```go
func zigzagFlattenFixed(block [blockSize][blockSize]int) [64]int {
    var coefficients [64]int
    for index, position := range zigzagOrder {
        coefficients[index] = block[position[0]][position[1]]
    }
    return coefficients
}
```

`range zigzagOrder` iterates the 64-element array, giving `(index, position)` pairs. `position[0]` is the row, `position[1]` is the column. This reads the 2D block in zigzag order and writes into a 1D array.

```go
func zigzagUnflatten(coefficients []int) [blockSize][blockSize]int {
    var block [blockSize][blockSize]int
    for index, position := range zigzagOrder {
        block[position[0]][position[1]] = coefficients[index]
    }
    return block
}
```

The inverse: reads the 1D sequence and places values back into the 2D 8×8 block.

---

## Color Space Helper Functions

### `extractYCbCrPlanes` — RGB to Three Float Planes

```go
func extractYCbCrPlanes(img image.Image) ([][]float64, [][]float64, [][]float64) {
    bounds := img.Bounds()
    // allocate three 2D slices
    for row := 0; row < height; row++ {
        for col := 0; col < width; col++ {
            r, g, b, _ := img.At(bounds.Min.X+col, bounds.Min.Y+row).RGBA()
            rf := float64(r >> 8)
            gf := float64(g >> 8)
            bf := float64(b >> 8)

            yPlane[row][col]  =  0.299*rf + 0.587*gf + 0.114*bf - 128.0
            cbPlane[row][col] = -0.168736*rf - 0.331264*gf + 0.5*bf
            crPlane[row][col] =  0.5*rf - 0.418688*gf - 0.081312*bf
        }
    }
    return yPlane, cbPlane, crPlane
}
```

`bounds.Min.X+col` — why add `bounds.Min`? Go images can have non-zero origins. A sub-image (crop) of a larger image might start at pixel (100, 200). Adding `bounds.Min` ensures we read from the correct starting position regardless of origin.

`r >> 8` — the `color.RGBA()` interface returns premultiplied 16-bit values. `>> 8` converts to 8-bit.

### `extractBlock` — 8×8 Block Extraction with Edge Clamping

```go
func extractBlock(plane [][]float64, startRow, startCol, planeHeight, planeWidth int) [blockSize][blockSize]float64 {
    var block [blockSize][blockSize]float64
    for row := 0; row < blockSize; row++ {
        for col := 0; col < blockSize; col++ {
            srcRow := startRow + row
            srcCol := startCol + col
            if srcRow >= planeHeight { srcRow = planeHeight - 1 }
            if srcCol >= planeWidth  { srcCol = planeWidth - 1  }
            block[row][col] = plane[srcRow][srcCol]
        }
    }
    return block
}
```

For interior blocks, the clamping never triggers. For edge blocks, some positions read the same edge pixel repeatedly. The returned value is a **value type** (`[8][8]float64`) — it is copied out of the function, not referenced. This is fine here because we immediately pass it to `ForwardDCT2D` which also takes a value type.

---

## Binary Packing — `packChannels`

```go
func packChannels(width, height, quality, subsampling int, yData, cbData, crData []int) []byte {
    yEncoded  := huffmanEncodeChannel(yData)
    cbEncoded := huffmanEncodeChannel(cbData)
    crEncoded := huffmanEncodeChannel(crData)

    const headerSize = 4 + 1 + 4 + 4 + 1 + 1 + 4 + 4 + 4  // = 27 bytes

    buf := make([]byte, 0, headerSize+len(yEncoded)+len(cbEncoded)+len(crEncoded))

    buf = append(buf, 'D', 'C', 'T', 0x02)                    // Magic bytes: "DCT\x02"
    buf = append(buf, 2)                                       // Version: 2
    buf = appendUint32LE(buf, uint32(width))                   // Width: 4 bytes LE
    buf = appendUint32LE(buf, uint32(height))                  // Height: 4 bytes LE
    buf = append(buf, byte(quality))                           // Quality: 1 byte
    buf = append(buf, byte(subsampling))                       // Subsampling: 1 byte
    buf = appendUint32LE(buf, uint32(len(yEncoded)))           // Y blob length: 4 bytes
    buf = appendUint32LE(buf, uint32(len(cbEncoded)))          // Cb blob length: 4 bytes
    buf = appendUint32LE(buf, uint32(len(crEncoded)))          // Cr blob length: 4 bytes
    buf = append(buf, yEncoded...)                             // Y Huffman data
    buf = append(buf, cbEncoded...)                            // Cb Huffman data
    buf = append(buf, crEncoded...)                            // Cr Huffman data

    return buf
}
```

**The 27-byte header**:
```
Offset  Size  Value
0       4     Magic: "DCT\x02"
4       1     Version: 2
5       4     Width (uint32, little-endian)
9       4     Height (uint32, little-endian)
13      1     Quality (0–100)
14      1     Chroma subsampling mode (0=444, 1=422, 2=420)
15      4     Y channel Huffman blob length (bytes)
19      4     Cb channel Huffman blob length (bytes)
23      4     Cr channel Huffman blob length (bytes)
27      —     Y Huffman data
—       —     Cb Huffman data
—       —     Cr Huffman data
```

**Why little-endian?** x86 CPUs are little-endian (least significant byte first). Go's `encoding/binary` package supports both; LE is the conventional choice for custom binary formats on mainstream hardware. The decoder reads these back with `binary.LittleEndian.Uint32(data[5:9])`.

**Magic bytes** `"DCT\x02"`: The first 4 bytes identify the file format. The decoder checks `string(data[0:3]) != "DCT"` as a validity check. `\x02` is version 2 (Huffman-encoded channels), distinguishing from a hypothetical version 1 that stored raw integers.

**Why separate blob lengths?** The decoder needs to know where each channel's data ends and the next begins. Without explicit lengths, the decoder would need a delimiter — which would require escaping valid byte sequences that look like delimiters. Explicit lengths are simpler and faster.

### `appendUint32LE`

```go
func appendUint32LE(buf []byte, v uint32) []byte {
    return append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
```

Decomposes a 32-bit integer into 4 bytes, least significant first. `byte(v)` keeps the lowest 8 bits; `byte(v>>8)` shifts right 8 and keeps the next 8 bits; etc.

### `ycbcrToRGB` — Used by Decoder

```go
func ycbcrToRGB(yVal, cbVal, crVal float64) color.RGBA {
    yVal += 128.0  // undo the level shift
    red   = yVal + 1.402*crVal
    green = yVal - 0.344136*cbVal - 0.714136*crVal
    blue  = yVal + 1.772*cbVal
    // clamp to [0, 255]
}
```

This lives in `encoder.go` but is used by the decoder. It is the exact mathematical inverse of the forward conversion. Note that after quantization and IDCT, the reconstructed Y/Cb/Cr values might be slightly outside their valid range; the clamp prevents invalid color values.

---

## Helper — `ceilDiv`

```go
func ceilDiv(a, b int) int {
    return (a + b - 1) / b
}
```

Integer ceiling division. Used to compute padded dimensions and block counts. Example: `ceilDiv(1081, 8) = (1081 + 7) / 8 = 1088 / 8 = 136`. The standard trick to avoid floating-point ceiling operations.

---

## Full Data Flow Through `compressChannel`

```
Input: plane [height][width]float64 (Y values, level-shifted)

1. Padding calculation → totalBlocks

2. For each block (parallel):
   extractBlock() → [8][8]float64 (with edge clamping)
       │
       ▼
   ForwardDCT2D() → [8][8]float64 (frequency coefficients)
       │
       ▼
   quantizeBlock() → [8][8]int (coarse integers)
       │
       ▼
   zigzagFlattenFixed() → [64]int (1D zigzag sequence)
       │
       ▼
   stored in processed[idx].zigzag

3. Serial (order matters):
   for each block in order:
     dcDelta = zz[0] - prevDC
     prevDC = zz[0]
     output: [dcDelta, ac1, ac2, ..., acK, eobMarker]

Output: []int (compact coefficient stream for this channel)
```

---

## Putting It Together — `Encode` Timeline

```
t=0ms   extractYCbCrPlanes()         — iterate all pixels, convert to YCbCr
t=2ms   subsampleChroma()            — average 2×2 regions for Cb/Cr
t=2ms   compressChannel(Y):
          worker pool launched
t=8ms     all blocks: DCT+quantize+zigzag (parallel)
t=8ms   wg.Wait()
t=9ms   serial DC delta + EOB
t=9ms   compressChannel(Cb): same (~0.25× the data)
t=12ms  compressChannel(Cr): same
t=14ms  huffmanEncodeChannel(Y): frequency count + tree build + encode
t=18ms  huffmanEncodeChannel(Cb)
t=20ms  huffmanEncodeChannel(Cr)
t=22ms  packChannels(): assemble header + blobs
t=22ms  return CompressionResult (encode complete)
```

The "179 ms" figure in benchmarks includes the subsequent decode + PSNR + SSIM calculation.

---

*Next: [Phase 06 — The Full Decode Pipeline](06_decode_pipeline.md)*
