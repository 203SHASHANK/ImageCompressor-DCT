# Phase 10 — Python Bridge & Benchmarking

> *DCTPress compares itself against Go stdlib JPEG, Python PIL, and Python OpenCV. This phase covers how Go spawns Python subprocesses, parses their JSON output, and coordinates the four-way benchmark.*

---

## Why Compare Against Python?

Go can call C libraries through CGO, and there are Go bindings for libjpeg. But the goal of benchmarking is to compare **industry-standard tools** against the custom implementation:

- **Python PIL (Pillow)**: the most widely-used Python image library; wraps libjpeg-turbo
- **Python OpenCV**: the computer vision library; also wraps libjpeg/libjpeg-turbo

Both are battle-tested, heavily optimized, and used in production. Measuring DCTPress against them puts its performance in concrete context.

---

## The Python Bridge Pattern

The challenge: Go and Python are different runtimes. You cannot call Python functions directly from Go (without embedding CPython, which is complex). Instead, DCTPress uses the **subprocess pattern**:

```
Go process                    Python process
    │                               │
    ├── exec.CommandContext(...)     │
    │       "python3 compress_pil.py --input /tmp/img.png --quality 75 --output-json"
    │                               │
    │                         compress image
    │                         compute PSNR/SSIM
    │                               │
    │    stdout: {"psnr": 33.7,...}  │
    │◄──────────────────────────────┤
    │                               │
    ├── json.Unmarshal(stdout)       │
    │── convert to BenchmarkResult  │
```

This is the "sidecar process" or "subprocess" pattern. It is used widely:
- Node.js calling Python ML models
- Ruby on Rails calling ImageMagick
- Any language calling FFmpeg

Advantages:
- No language binding complexity
- Each process runs in its own memory space (crashes are isolated)
- Easy to test the scripts independently with the command line

Disadvantages:
- Process startup overhead (~50–100 ms for Python interpreter startup)
- Data transfer via stdin/stdout/files (no shared memory)
- JSON serialization/deserialization overhead

For benchmarking (not hot path), these overheads are acceptable.

---

## `bridge.go` — The Go Side

### The Bridge Struct

```go
type Bridge struct {
    pythonExecutable string  // "python3" on Unix, "python" on Windows
    scriptsDirectory string  // "internal/python/scripts"
}

func NewBridge() *Bridge {
    return &Bridge{
        pythonExecutable: detectPython(),
        scriptsDirectory: resolveScriptsDirectory(),
    }
}
```

`detectPython()` returns the appropriate Python binary name for the OS:

```go
func detectPython() string {
    if runtime.GOOS == "windows" {
        return "python"   // Windows uses "python" not "python3"
    }
    return "python3"      // Unix/macOS
}
```

`resolveScriptsDirectory()` returns the scripts path relative to the working directory:

```go
func resolveScriptsDirectory() string {
    return filepath.Join("internal", "python", "scripts")
}
```

This works when the server is run from the project root (`go run cmd/server/main.go`). It also works in the Docker container because the scripts are copied to the same relative path.

### Availability Check

```go
func (bridge *Bridge) IsAvailable() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    err := exec.CommandContext(ctx, bridge.pythonExecutable, "--version").Run()
    return err == nil
}
```

Before attempting any Python benchmark, the bridge checks if Python is available. If Python is not installed (e.g., in a minimal deployment), the Python benchmarks are silently skipped and only Go benchmarks are returned.

The 5-second timeout prevents the availability check from hanging if Python is somehow stuck.

### Running a Script — `runScript`

```go
func (bridge *Bridge) runScript(scriptName, imagePath string, quality int) (models.BenchmarkResult, error) {
    scriptPath := filepath.Join(bridge.scriptsDirectory, scriptName)

    ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
    defer cancel()

    cmd := exec.CommandContext(ctx,
        bridge.pythonExecutable,
        scriptPath,
        "--input", imagePath,
        "--quality", fmt.Sprintf("%d", quality),
        "--output-json",
    )

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        return models.BenchmarkResult{}, fmt.Errorf(
            "script %q failed: %w\nstderr: %s", scriptName, err, stderr.String())
    }
    
    var rawResult struct {
        Method           string  `json:"method"`
        CompressedSize   int     `json:"compressed_size"`
        CompressionRatio float64 `json:"compression_ratio"`
        EncodingTimeMS   float64 `json:"encoding_time_ms"`
        DecodingTimeMS   float64 `json:"decoding_time_ms"`
        PSNR             float64 `json:"psnr"`
        SSIM             float64 `json:"ssim"`
    }

    if err := json.Unmarshal(stdout.Bytes(), &rawResult); err != nil {
        return models.BenchmarkResult{}, fmt.Errorf(
            "failed to parse output from %q: %w", scriptName, err)
    }

    return models.BenchmarkResult{
        EncodingTime: time.Duration(rawResult.EncodingTimeMS * float64(time.Millisecond)),
        ...
    }, nil
}
```

**`exec.CommandContext(ctx, ...)`**: Creates a command object. Arguments are passed as separate strings — no shell interpolation, no command injection risk. The `ctx` cancels the command if it exceeds `scriptTimeout` (60 seconds).

**Separate stdout and stderr**: `cmd.Stdout = &stdout` captures normal output. `cmd.Stderr = &stderr` captures error output. This separation is critical: if Python prints a warning to stderr, it would corrupt the JSON if stderr were mixed with stdout. The stderr content is included in error messages for debugging.

**`scriptTimeout = 60 * time.Second`**: A 60-second timeout for each Python script. Processing a large 1080p image through PIL might take several seconds; 60 seconds is generous.

**`time.Duration(rawResult.EncodingTimeMS * float64(time.Millisecond))`**: The Python script reports timing in floating-point milliseconds. Go's `time.Duration` is in nanoseconds. `time.Millisecond = 1,000,000 nanoseconds`. So `5.3 ms × 1,000,000 ns/ms = 5,300,000 ns = 5.3 ms` as a `time.Duration`.

---

## Temporary File for Image Transfer

```go
// benchmark_service.go
func saveTempPNG(img image.Image) (path string, cleanup func(), err error) {
    tempFile, err := os.CreateTemp("", "imgcomp_bench_*.png")
    if err != nil {
        return "", nil, fmt.Errorf("create temp file: %w", err)
    }

    if err := png.Encode(tempFile, img); err != nil {
        tempFile.Close()
        os.Remove(tempFile.Name())
        return "", nil, fmt.Errorf("encode temp PNG: %w", err)
    }
    tempFile.Close()

    name := tempFile.Name()
    return name, func() { os.Remove(name) }, nil
}
```

The image is passed to Python as a PNG file. Why PNG and not JPEG?

- PNG is **lossless** — the Python script receives the exact same image data as Go processed
- If JPEG were used for transfer, the Python script would measure quality loss relative to an already-JPEG-compressed input, making comparison unfair
- Both PIL and OpenCV support PNG as input

`os.CreateTemp("", "imgcomp_bench_*.png")` creates a temp file in the default temp directory. The `*` is replaced with a random string. The returned file is already opened.

The cleanup function `func() { os.Remove(name) }` is returned to the caller, who `defer cleanup()` it. This ensures the temp file is deleted even if the benchmarks fail.

---

## `BenchmarkService.RunAll` — Coordination

```go
func (bs *BenchmarkService) RunAll(img image.Image, quality int) []models.BenchmarkResult {
    var results []models.BenchmarkResult

    // 1. Custom DCT (Go)
    if result, err := bs.runCustomDCT(img, quality); err == nil {
        results = append(results, result)
    } else {
        log.Printf("Custom DCT benchmark failed: %v", err)
    }

    // 2. Go stdlib JPEG
    if result, err := CompressGoJPEG(img, quality); err == nil {
        results = append(results, result)
    } else {
        log.Printf("Go JPEG benchmark failed: %v", err)
    }

    // 3 & 4. Python PIL + OpenCV (optional)
    if bs.pythonBridge.IsAvailable() {
        tempPath, cleanup, err := saveTempPNG(img)
        if err == nil {
            defer cleanup()

            if result, err := bs.pythonBridge.RunPILBenchmark(tempPath, quality); err == nil {
                results = append(results, result)
            }
            if result, err := bs.pythonBridge.RunOpenCVBenchmark(tempPath, quality); err == nil {
                results = append(results, result)
            }
        }
    }

    return results
}
```

**Non-fatal failures**: Each method's error is logged but does not abort the benchmark. If Python fails, the Go results are still returned. The API response might have 2 results (only Go codecs) or 4 results (all codecs).

**Single temp file for both Python benchmarks**: The temp PNG is created once and both `compress_pil.py` and `compress_opencv.py` read from it. This avoids creating the same file twice.

**Sequential execution**: The benchmarks run one after another (not in parallel). This gives each codec access to the full CPU and produces more reproducible timing measurements. Parallel execution would introduce contention and make timing comparisons unreliable.

---

## Python Scripts — `compress_pil.py`

```python
def compress_with_pil(input_path: str, quality: int) -> dict:
    import numpy as np
    from PIL import Image
    from skimage.metrics import peak_signal_noise_ratio, structural_similarity

    output_path = "/tmp/benchmark_pil_output.jpg"

    original = Image.open(input_path).convert("RGB")
    original_np = np.array(original)

    encode_start = time.perf_counter()
    original.save(output_path, "JPEG", quality=quality, optimize=True)
    encoding_time_ms = (time.perf_counter() - encode_start) * 1000

    decode_start = time.perf_counter()
    compressed = Image.open(output_path).convert("RGB")
    decoding_time_ms = (time.perf_counter() - decode_start) * 1000

    compressed_np = np.array(compressed)
    psnr = peak_signal_noise_ratio(original_np, compressed_np, data_range=255)
    ssim = structural_similarity(original_np, compressed_np, channel_axis=2, data_range=255)

    width, height = original.size
    raw_size = width * height * 3   # uncompressed RGB bytes (same baseline as Go)
    compressed_size = os.path.getsize(output_path)

    return { "method": "Python PIL", "psnr": float(psnr), "ssim": float(ssim), ... }
```

**`convert("RGB")`**: Pillow might decode to RGBA or other modes. Converting to RGB ensures consistent 3-channel data for the numpy array and metric calculations.

**`optimize=True`**: PIL's JPEG optimize flag uses Huffman table optimization (similar to DCTPress's adaptive Huffman). Without this, PIL uses default Huffman tables. With it, PIL's file sizes are smaller.

**`raw_size = width * height * 3`**: Same calculation as Go's `originalSize`. Both use raw RGB bytes as the denominator for compression ratio, ensuring fair comparison.

**`time.perf_counter()`**: Python's highest-resolution timer. Returns a float in seconds. The difference multiplied by 1000 gives milliseconds.

**`channel_axis=2`**: The `structural_similarity` function in scikit-image 0.19+ requires `channel_axis` instead of the deprecated `multichannel` parameter. This was a bug fix from an earlier version.

### `compress_opencv.py`

```python
def compress_with_opencv(input_path: str, quality: int) -> dict:
    import cv2
    
    original_bgr = cv2.imread(input_path)
    original_rgb = cv2.cvtColor(original_bgr, cv2.COLOR_BGR2RGB)  # OpenCV uses BGR!

    encode_start = time.perf_counter()
    cv2.imwrite(output_path, original_bgr, [cv2.IMWRITE_JPEG_QUALITY, quality])
    encoding_time_ms = (time.perf_counter() - encode_start) * 1000
    
    compressed_bgr = cv2.imread(output_path)
    compressed_rgb = cv2.cvtColor(compressed_bgr, cv2.COLOR_BGR2RGB)
    
    psnr = peak_signal_noise_ratio(original_rgb, compressed_rgb, data_range=255)
    ssim = structural_similarity(original_rgb, compressed_rgb, channel_axis=2, data_range=255)
```

**BGR vs RGB**: OpenCV stores images in BGR (Blue-Green-Red) order, not RGB. This is a historical quirk from when OpenCV was primarily for video processing where BGR was common. The `cv2.cvtColor(img, cv2.COLOR_BGR2RGB)` conversion is essential for correct PSNR/SSIM calculation — swapped channels would give wrong metric values.

**`cv2.IMWRITE_JPEG_QUALITY`**: OpenCV's JPEG quality parameter. Same range (1–100) as PIL and DCTPress, using libjpeg's quality interpretation.

---

## Benchmark Results Interpretation

From the README (quality=75, 1920×1080):

| Codec | File Size | Ratio | PSNR | SSIM | Encode Time |
|-------|-----------|-------|------|------|-------------|
| **DCTPress** | **83.4 KB** | **48.3×** | 33.77 dB | 0.9924 | 179 ms |
| Go stdlib JPEG | 86.9 KB | 46.4× | 33.84 dB | 0.9934 | 29 ms |
| Python PIL | 76.8 KB | — | 33.70 dB | 0.9822 | 7 ms |
| Python OpenCV | 86.7 KB | — | 33.70 dB | 0.9822 | 2 ms |

**DCTPress vs Go stdlib JPEG**: 4% smaller file, essentially same PSNR (0.07 dB difference is imperceptible). Trade-off: 6× slower encoding (179 ms vs 29 ms including decode+metrics; raw encoding ~40 ms vs 29 ms).

**Go stdlib vs OpenCV**: Nearly identical file sizes (both use libjpeg internally) and quality metrics. The 2 ms vs 29 ms difference is libjpeg-turbo's SIMD acceleration (OpenCV) vs Go's pure-Go JPEG encoder.

**PIL ratios "—"**: The README notes PIL's ratio is not directly comparable because PIL measures compression relative to the uploaded file (which may already be compressed), not raw RGB. DCTPress and Go stdlib always use raw RGB as the baseline.

**179 ms for DCTPress**: This is the full cycle (encode + decode + PSNR + SSIM). Encode alone is ~40 ms. The decode adds ~15 ms, PSNR + SSIM add ~120 ms (pixel iteration over a 1080p image is not free).

---

## Error Handling Strategy

The Python bridge has a graceful degradation design:

```
IsAvailable() fails → skip all Python benchmarks silently
saveTempPNG() fails → log error, skip Python benchmarks
script fails (crash, ImportError, timeout) → log error, skip that codec's result
JSON parse fails → log error, skip that codec's result
```

In all failure cases, the Go benchmark results are still returned. The API response will have fewer results but is never empty (the custom DCT always runs).

This makes the server robust in environments where Python is not available (e.g., minimal Docker containers, test environments).

---

*Next: [Phase 11 — Testing Strategy](11_testing_strategy.md)*
