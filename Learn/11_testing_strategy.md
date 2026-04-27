# Phase 11 — Testing Strategy

> *A codec is only as good as its ability to verify its own correctness. DCTPress has three test layers — unit tests for algorithms, integration tests for HTTP endpoints, and benchmarks for performance measurement.*

---

## Test Layout

```
test/
├── unit/
│   ├── dct/
│   │   ├── dct_test.go        ← DCT math, encoder/decoder round-trips
│   │   └── roundtrip_test.go  ← End-to-end quality and edge cases
│   ├── huffman/
│   │   └── huffman_test.go    ← Huffman encode/decode correctness
│   └── metrics/
│       └── metrics_test.go    ← PSNR, SSIM, MSE correctness
└── integration/
    └── api_test.go            ← Full HTTP request/response cycle
```

Run all tests: `make test` (expands to `go test -v -cover ./...`)
Run with race detector: `make test-race`
Run benchmarks: `make benchmark`

---

## Go Testing Fundamentals

```go
package dct_test  // "_test" suffix = external test package (black-box testing)

import "testing"

func TestXxx(t *testing.T) {  // must start with Test
    // ...
    t.Errorf("message: got %v, want %v", got, want)  // mark failed, continue
    t.Fatalf("message")                               // mark failed, stop this test
    t.Logf("message: %v", value)                     // log (only shown on failure)
}

func BenchmarkXxx(b *testing.B) {  // must start with Benchmark
    b.ResetTimer()
    for i := 0; i < b.N; i++ {   // b.N is set by the testing framework
        // code to benchmark
    }
}
```

**`package dct_test` vs `package dct`**: Using the `_test` suffix creates an **external test package** that can only access exported symbols (capitalized names). This enforces that the test only uses the public API, not implementation details. It's the Go equivalent of black-box testing.

**`t.Errorf` vs `t.Fatalf`**: `Errorf` marks the test as failed but continues running the test function. `Fatalf` marks failed and immediately stops. Use `Fatalf` for errors that would cause panic/nil pointer in subsequent code; use `Errorf` for independent checks.

---

## DCT Unit Tests — `dct_test.go`

### Mathematical Property Tests

```go
func TestForwardDCT2D_ZeroBlock(t *testing.T) {
    var zeroBlock [8][8]float64
    result := dct.ForwardDCT2D(zeroBlock)
    for u := 0; u < 8; u++ {
        for v := 0; v < 8; v++ {
            if result[u][v] != 0 {
                t.Errorf("expected 0 at [%d][%d], got %f", u, v, result[u][v])
            }
        }
    }
}
```

**What this tests**: Linearity property — DCT of zero input must be zero output. If the normalization constants or cosine lookup table have a bug, this might fail.

```go
func TestForwardInverseDCT_RoundTrip(t *testing.T) {
    var original [8][8]float64
    for row := 0; row < 8; row++ {
        for col := 0; col < 8; col++ {
            original[row][col] = float64(row*8+col) - 32.0
        }
    }
    dctBlock := dct.ForwardDCT2D(original)
    reconstructed := dct.InverseDCT2D(dctBlock)

    for row := 0; row < 8; row++ {
        for col := 0; col < 8; col++ {
            diff := math.Abs(original[row][col] - reconstructed[row][col])
            if diff > 1e-9 {
                t.Errorf("round-trip mismatch at [%d][%d]: diff=%.2e", row, col, diff)
            }
        }
    }
}
```

**What this tests**: IDCT(DCT(x)) = x with floating-point precision tolerance of 1e-9. This validates:
1. The normalization factor `0.5 * alphaFactor(k)` is correct
2. The precomputed cosine values are accurate
3. The separable 2-pass approach is mathematically equivalent to the direct formula

**Why `1e-9` tolerance?** Floating-point arithmetic is not exact. Multiplying 64 floating-point numbers and then reversing the operation accumulates rounding errors. The theoretical error bound for 8-point DCT round-trip is O(machine epsilon × N × coefficient range) ≈ 10⁻¹⁵ × 64 × 100 ≈ 10⁻¹¹. The 10⁻⁹ tolerance has a 2-order-of-magnitude safety margin.

```go
func TestForwardDCT2D_ConstantBlock(t *testing.T) {
    // all pixels = 100
    result := dct.ForwardDCT2D(constantBlock)
    if result[0][0] == 0 {
        t.Error("DC coefficient should be non-zero for constant block")
    }
    for u := 0; u < 8; u++ {
        for v := 0; v < 8; v++ {
            if u == 0 && v == 0 { continue }
            if math.Abs(result[u][v]) > 1e-9 {
                t.Errorf("AC coefficient [%d][%d] should be ~0 for constant block", u, v)
            }
        }
    }
}
```

**What this tests**: Energy compaction — all energy from a constant block must concentrate in DC. Any AC energy in a constant block indicates a bug in the orthogonality of the transform.

### Encoder/Decoder Round-Trip

```go
func TestEncoderDecoder_RoundTrip(t *testing.T) {
    img := createSolidColorImage(64, 64, color.RGBA{R: 200, G: 100, B: 50, A: 255})

    encoder, err := dct.NewEncoder(dct.CompressionOptions{
        Quality:           90,
        ChromaSubsampling: dct.Subsampling444,
    })
    result, err := encoder.Encode(img)
    // checks: no error, CompressedSize > 0, CompressionRatio > 0
    
    decoder := dct.NewDecoder()
    decoded, err := decoder.Decode(result.Data)
    // checks: no error, dimensions match 64×64
}
```

**What this tests**: The complete compression pipeline including Huffman coding, binary packing, and decoding. Does the full encode→decode cycle produce an output of the correct dimensions without errors?

This test does not check quality (PSNR/SSIM) — that's the job of `roundtrip_test.go`.

```go
func TestNewEncoder_InvalidQuality(t *testing.T) {
    _, err := dct.NewEncoder(dct.CompressionOptions{Quality: 0})
    if err == nil { t.Error("expected error for quality=0") }

    _, err = dct.NewEncoder(dct.CompressionOptions{Quality: 101})
    if err == nil { t.Error("expected error for quality=101") }
}
```

**Boundary condition testing**: quality=0 and quality=101 must return errors. These test the input validation in `NewEncoder`.

### Benchmarks in the Unit Test File

```go
func BenchmarkForwardDCT2D(b *testing.B) {
    var block [8][8]float64
    // ... fill block ...
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        dct.ForwardDCT2D(block)
    }
}

func BenchmarkEncode1080p(b *testing.B) {
    img := createSolidColorImage(1920, 1080, ...)
    encoder, _ := dct.NewEncoder(...)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        encoder.Encode(img)
    }
}
```

`b.ResetTimer()` — starts the benchmark timer after setup. Setup code (creating the image, creating the encoder) is not counted in benchmark time.

`b.N` — the testing framework automatically adjusts this to run the benchmark long enough to get a stable measurement. Typical runs: 10 to 1,000,000 iterations depending on how fast the code is.

Run: `go test -bench=BenchmarkForwardDCT2D -benchmem ./test/unit/dct/`

`-benchmem` reports allocations per operation. For `BenchmarkForwardDCT2D`, you should see 0 allocations (the function works on stack-allocated arrays).

---

## Round-Trip Quality Tests — `roundtrip_test.go`

These tests go beyond "does it work" to "how well does it work":

```go
func TestRoundTrip_Gradient(t *testing.T) {
    img := image.NewRGBA(image.Rect(0, 0, 64, 64))
    // ... fill with gradient ...
    
    enc, _ := dct.NewEncoder(dct.CompressionOptions{Quality: 75, ChromaSubsampling: dct.Subsampling420})
    result, _ := enc.Encode(img)
    decoded, err := dct.NewDecoder().Decode(result.Data)
    
    psnr := pixelPSNR(img, decoded)
    t.Logf("Q75 4:2:0 gradient 64×64: PSNR=%.2f dB  compressed=%d bytes", psnr, result.CompressedSize)
    if psnr < 35 {
        t.Errorf("PSNR %.2f dB too low for gradient image", psnr)
    }
}
```

**PSNR threshold = 35 dB for gradients**: A smooth gradient is low-frequency content — exactly what DCT handles best. Below 35 dB for a gradient suggests a serious quantization bug.

```go
func TestRoundTrip_Sinusoidal(t *testing.T) {
    img := makeSinusoidalImage(320, 240)
    for _, q := range []int{50, 75, 90} {
        // encode/decode at each quality
        psnr := pixelPSNR(img, decoded)
        if psnr < 28 {
            t.Errorf("Q%d: PSNR %.2f dB too low for sinusoidal image", q, psnr)
        }
    }
}
```

**Why sinusoidal test image?** A sinusoidal image has energy at known frequencies. It simulates natural image content better than a solid color or linear gradient. Testing at multiple quality levels (50, 75, 90) catches quality scaling bugs.

**PSNR threshold = 28 dB**: Lower threshold because sinusoidal content has more high-frequency energy that gets more aggressively quantized.

```go
func TestRoundTrip_EOBEdgeCase(t *testing.T) {
    // high-frequency image at Quality 100
    // Q100 minimizes quantization → more non-zero ACs → EOB is written after position 62
    // this tests the boundary: what happens when all 63 AC slots are filled?
    psnr := pixelPSNR(img, decoded)
    if psnr < 40 { t.Errorf("Q100 round-trip PSNR too low: %.2f", psnr) }
}
```

**EOB edge case**: The comment says "Q100 encodes with minimal quantization, maximizing surviving ACs." This creates blocks where the EOB marker appears at position 63 (the last position), or where the encoder writes all 63 AC values without an EOB (though in practice, the encoder always writes the EOB after the last non-zero value). This tests that the decoder correctly handles these dense blocks and that the next block's DC delta is read from the correct position.

### The `pixelPSNR` Helper

```go
func pixelPSNR(orig, dec image.Image) float64 {
    b := orig.Bounds()
    mse := 0.0
    n := float64(b.Dx() * b.Dy() * 3)
    for y := b.Min.Y; y < b.Max.Y; y++ {
        for x := b.Min.X; x < b.Max.X; x++ {
            or, og, ob, _ := orig.At(x, y).RGBA()
            dr, dg, db, _ := dec.At(x, y).RGBA()
            d := func(a, b uint32) float64 { v := float64(a>>8) - float64(b>>8); return v * v }
            mse += d(or, dr) + d(og, dg) + d(ob, db)
        }
    }
    mse /= n
    if mse == 0 { return math.Inf(1) }
    return 10 * math.Log10(255*255/mse)
}
```

This is a local reimplementation of PSNR for the test file, avoiding a dependency on the metrics package. It is functionally identical to `metrics.CalculatePSNR`. The closure `d := func(a, b uint32) float64 { ... }` avoids code duplication for the three channels.

---

## Huffman Tests — `huffman_test.go`

```go
func TestHuffman_RoundTrip_SimpleSequence(t *testing.T) {
    coefficients := []int{0, 0, 1, -1, 0, 2, 0, 0, 3, -2, 0, 0, 0, 1}
    encoder := huffman.NewEncoder(coefficients)
    encoded, _ := encoder.Encode(coefficients)
    decoded, _ := huffman.Decode(encoded)
    // verify decoded == coefficients (element by element)
}
```

Every Huffman test follows the same pattern: encode → decode → compare. Together they cover:

- **Simple sequence**: typical mixed values
- **All zeros**: the single-symbol fast path (`uniqueCount == 1`)
- **Single symbol**: `{42, 42, 42, ...}` — the fast path stores just [count]
- **Negative coefficients**: signed values, crucial for delta-coded DC and AC
- **Large block**: simulates a realistic 640-integer stream with sparse non-zeros
- **Compression ratio**: verifies Huffman actually compresses vs raw int16

```go
func TestHuffman_CompressionRatio(t *testing.T) {
    coefficients := make([]int, 640)
    coefficients[0] = 100     // only 2 non-zero values
    coefficients[64] = 80
    
    encoder := huffman.NewEncoder(coefficients)
    encoded, _ := encoder.Encode(coefficients)

    rawSize := len(coefficients) * 2  // int16 per coefficient
    if len(encoded) >= rawSize {
        t.Errorf("Huffman should be smaller than raw int16: encoded=%d raw=%d", len(encoded), rawSize)
    }
}
```

This is a **semantic test** — not just "does it round-trip" but "does it actually compress?" A sparse stream (638 zeros + 2 non-zeros) must be smaller than raw int16 representation. If the Huffman implementation is broken in a way that inflates data, this catches it.

---

## Integration Tests — `api_test.go`

Integration tests test the full HTTP layer:

```go
func TestUploadHandler_ValidPNG(t *testing.T) {
    pngData := buildTestPNG(64, 64)
    body, contentType := buildMultipartUpload(pngData)

    req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
    req.Header.Set("Content-Type", contentType)
    rec := httptest.NewRecorder()

    handlers.UploadHandler(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }
    // decode and verify JSON response
}
```

**`httptest.NewRequest`** — creates an HTTP request without sending it over the network.
**`httptest.NewRecorder`** — captures the HTTP response (status code, headers, body) without a real connection.

This testing approach (using `httptest`) is idiomatic Go. It tests the handler functions directly without starting a real server, making tests fast and isolated.

```go
func TestCompressHandler_FullFlow(t *testing.T) {
    // Step 1: upload
    // get imageID from upload response
    
    // Step 2: compress using imageID
    compressPayload, _ := json.Marshal(map[string]interface{}{
        "image_id": imageID,
        "quality": 75,
        "chroma_subsampling": "4:2:0",
    })
    // verify compress response has compression_ratio, psnr, ssim
}
```

**Multi-step integration tests**: Real users do upload → then compress → then download. The integration test simulates this entire flow. Each step builds on the previous step's response.

**Why not just unit test the service?** The handler has its own logic: JSON parsing, error response format, image store, download store. These must be tested too. Unit testing the service alone wouldn't catch bugs in how the handler calls the service or formats the response.

```go
func TestCompressHandler_InvalidQuality(t *testing.T) {
    // quality = 0 → expect 400 Bad Request
}

func TestCompressHandler_UnknownImageID(t *testing.T) {
    // image_id = "nonexistent" → expect 404 Not Found
}
```

**Error path coverage**: Happy path tests are insufficient. These tests verify that the handler returns appropriate HTTP status codes for invalid inputs.

---

## Test Image Helpers

Both `dct_test.go` and `api_test.go` define helpers for creating test images:

```go
// dct_test.go
func createSolidColorImage(width, height int, c color.RGBA) image.Image {
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            img.SetRGBA(x, y, c)
        }
    }
    return img
}

// api_test.go
func buildTestPNG(width, height int) []byte {
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            img.SetRGBA(x, y, color.RGBA{
                R: uint8(x % 256),
                G: uint8(y % 256),
                B: 128,
                A: 255,
            })
        }
    }
    var buf bytes.Buffer
    png.Encode(&buf, img)
    return buf.Bytes()
}
```

`buildTestPNG` creates a PNG with a gradient pattern — different colors at different positions. This gives the encoder real work to do (not just a constant color) while being deterministic.

---

## Test Coverage

`make coverage` runs:
```bash
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

This produces an HTML report showing which lines were executed during tests. Lines in red were not hit by any test.

Key areas with high coverage in DCTPress:
- `transform.go`: DCT math tested by unit and round-trip tests
- `quantization.go`: tested via encoder/decoder tests
- `huffman/encoder.go`: tested by all Huffman tests
- `handlers.go`: tested by integration tests

---

## Race Detector — `make test-race`

```bash
go test -race ./...
```

The Go race detector instruments memory accesses and detects **data races** — concurrent reads and writes to shared memory without synchronization. For DCTPress, potential races are:

- The `imageStore` map (protected by `imageStoreMu`)
- The `processed` slice in the parallel block loop (each goroutine writes different indices — safe)
- The `workChan` channel (channels are inherently race-free)

Running with `-race` in tests catches any mutex bugs or missed synchronization points. The race detector adds ~5–10× overhead; don't use it for benchmarks.

---

## What Makes a Good Codec Test?

For comparison, here are the properties of DCTPress's test suite:

| Property | DCTPress |
|----------|---------|
| Unit tests for core math | ✓ (DCT properties) |
| Round-trip quality thresholds | ✓ (PSNR > 35 dB for gradients) |
| Edge cases (EOB, single-symbol Huffman) | ✓ |
| Compression effectiveness | ✓ (Huffman must actually compress) |
| Error handling (bad inputs) | ✓ (invalid quality, unknown ID) |
| Full HTTP flow | ✓ (upload → compress → download) |
| Benchmarks | ✓ (ForwardDCT2D, Encode1080p, HuffmanEncode) |
| Race detection | ✓ (via make test-race) |

---

*Next: [Phase 12 — Infrastructure: Docker, Makefile, and Deployment](12_infrastructure.md)*
