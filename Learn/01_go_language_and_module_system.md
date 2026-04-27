# Phase 01 — Go Language & Module System

> *DCTPress is written in Go. Before diving into algorithms, you need to understand the language features and project conventions that appear everywhere in the code.*

---

## Why Go?

Go (also called Golang) was created at Google in 2009. Its design goals are:

1. **Fast compilation** — the entire project compiles in under 2 seconds
2. **Simple concurrency** — goroutines and channels make parallel work intuitive
3. **Zero-dependency stdlib** — HTTP server, JSON, image codecs, all built-in
4. **Deterministic memory** — garbage collector, no manual malloc/free
5. **Static typing** — errors caught at compile time, not runtime

For a codec project, Go's strengths are:
- Goroutines for parallel block processing (Phase 05)
- `image.Image` interface in stdlib (every codec speaks the same language)
- `net/http` for the REST API with zero third-party dependencies
- Performance close enough to C++ for this use case (~40 ms to encode 1080p)

---

## The Module System — `go.mod`

```
# go.mod (root of the project)
module imagecompressor-dct

go 1.25.0

require golang.org/x/image v0.39.0
```

**`module imagecompressor-dct`** — This is the **module name**. It is the root import path for every package in the project. When any file inside the project writes:

```go
import "imagecompressor-dct/internal/core/dct"
```

Go resolves this to the `internal/core/dct/` directory relative to the `go.mod` file.

**`go 1.25.0`** — The minimum Go version required to build this project.

**`require golang.org/x/image v0.39.0`** — The only external dependency. `golang.org/x/image` is the "extended image" library maintained by the Go team. It provides decoders for BMP, TIFF, and WebP formats that are not in the standard library.

> **Real-World Analogy**: `go.mod` is like a recipe card. The module name is the dish's name, and the `require` section lists ingredients you need to fetch. When you run `go mod tidy`, Go fetches the exact versions and records their checksums in `go.sum` — like noting which brand of flour you used so the recipe is reproducible.

---

## Packages — The Unit of Code Organization

Every `.go` file starts with a `package` declaration. All `.go` files in the same directory must have the same package name (with one exception: `_test` suffix for test files).

### Package map in DCTPress

| Directory | Package name | Purpose |
|-----------|-------------|---------|
| `cmd/server/` | `main` | Entry point — `func main()` lives here |
| `internal/core/dct/` | `dct` | Transform, quantization, encoder, decoder |
| `internal/core/huffman/` | `huffman` | Entropy coding |
| `internal/core/metrics/` | `metrics` | PSNR, SSIM, MSE |
| `internal/handlers/` | `handlers` | HTTP handlers |
| `internal/models/` | `models` | Shared data types |
| `internal/services/` | `services` | Business logic orchestration |
| `internal/io/` | `io` | File I/O utilities |
| `internal/python/` | `python` | Python subprocess bridge |

### The `internal/` convention

Go has a special rule: packages inside a directory named `internal` can **only** be imported by code rooted at the parent of `internal`. 

In this project, all the codec logic is under `internal/`. This means:
- Code in `cmd/server/main.go` can import `internal/handlers`
- An external project **cannot** import `imagecompressor-dct/internal/core/dct`

This is intentional — the core codec is implementation detail, not a public API.

---

## Key Go Keywords and Constructs Used in DCTPress

### `package` and `import`

```go
package dct

import (
    "fmt"
    "image"
    "image/color"
    "imagecompressor-dct/internal/core/huffman"
    "imagecompressor-dct/internal/models"
    "runtime"
    "sync"
)
```

Grouped imports: stdlib packages first, then external, then internal (by convention). The blank identifier `_` in `import _ "image/jpeg"` means "import for side effects only" — it runs the package's `init()` function which registers the JPEG decoder, but the package name is not used directly.

### `const` — Compile-Time Constants

```go
// transform.go
const blockSize = 8

// encoder.go
const eobMarker = -32768

// handlers.go
const maxUploadBytes = 10 * 1024 * 1024  // 10 MB
```

Constants are evaluated at compile time. `blockSize = 8` appears 40+ times across the codebase as array dimensions, loop bounds, and arithmetic — defining it once prevents bugs from typos and makes it clear this `8` is not magic.

### `var` — Package-Level Variables

```go
// transform.go
var precomputedCosines [blockSize][blockSize]float64

// encoder.go
var zigzagOrder = [64][2]int{ ... }

// handlers.go
var (
    imageStore   = make(map[string]imageEntry)
    imageStoreMu sync.RWMutex
    imageIDSeq   int
)
```

Package-level `var` declarations are initialized once when the package is loaded. They persist for the lifetime of the program.

### `init()` — Package Initialization

```go
// transform.go
func init() {
    for u := 0; u < blockSize; u++ {
        for x := 0; x < blockSize; x++ {
            precomputedCosines[u][x] = math.Cos((2*float64(x)+1) * float64(u) * math.Pi / 16.0)
        }
    }
}
```

`init()` runs automatically before `main()`, once per package, in the order packages are imported. Here it precomputes 64 cosine values so the DCT loop never calls `math.Cos` at runtime. This is a standard Go pattern for expensive one-time setup.

### `type` — Custom Type Definitions

```go
// encoder.go
type ChromaSubsampling int

const (
    Subsampling444 ChromaSubsampling = iota  // 0
    Subsampling422                           // 1
    Subsampling420                           // 2
)
```

`iota` is a Go built-in that auto-increments for each `const` in a block, starting at 0. This creates an enumeration of subsampling modes as typed integers — the compiler will catch if you accidentally pass a raw `int` where a `ChromaSubsampling` is expected.

### `struct` — Composite Data Types

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

A struct groups related fields. `Encoder` holds the configuration and precomputed tables for one compression session. Notice lowercase field names (`options`, `luminanceQuantTable`) — lowercase means unexported (private to the package). Uppercase (`Quality`, `ChromaSubsampling`) means exported (accessible outside the package).

### Methods — Functions with Receivers

```go
// encoder.go
func (encoder *Encoder) Encode(img image.Image) (*models.CompressionResult, error) {
    // ...
}
```

`(encoder *Encoder)` is the **receiver** — it attaches this function to the `Encoder` type. `*Encoder` (pointer receiver) means the method can modify the struct and avoids copying it. You call this as `encoder.Encode(img)`.

### Interfaces — Behavioral Contracts

```go
// From Go stdlib
type Image interface {
    ColorModel() color.Model
    Bounds() Rectangle
    At(x, y int) color.Color
}
```

Go interfaces are implicit — any type that implements all the methods of an interface satisfies it. `image.Image` is the universal image type in Go. Every codec (PNG decoder, JPEG decoder, DCTPress encoder) works with `image.Image`. This is why DCTPress can accept any format without special-casing.

> **Real-World Analogy**: An interface is like an electrical outlet standard. Any appliance (device) that has the right plug shape (implements the interface) can use the outlet (be passed to a function). You don't need to know if it's a hair dryer or a laptop.

### Goroutines and Channels — Concurrency

```go
// encoder.go (parallel block processing)
workChan := make(chan blockWork, totalBlocks)
var wg sync.WaitGroup

for w := 0; w < numWorkers; w++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        for work := range workChan {
            // process one block
        }
    }()
}
```

- `go func()` launches a **goroutine** — a lightweight thread managed by Go's runtime (not OS threads)
- `chan blockWork` is a **channel** — a pipe for sending values between goroutines
- `sync.WaitGroup` tracks when all goroutines finish
- `defer wg.Done()` runs `wg.Done()` when the goroutine function returns, even if it panics

This pattern is the Go way to implement a **worker pool** — a fixed number of goroutines that each pull work off a shared queue.

### `sync.RWMutex` — Concurrent Map Access

```go
// handlers.go
var (
    imageStore   = make(map[string]imageEntry)
    imageStoreMu sync.RWMutex
)

func storeImage(img image.Image, fileSize int) string {
    imageStoreMu.Lock()          // exclusive write lock
    defer imageStoreMu.Unlock()  // released when function returns
    imageIDSeq++
    id := fmt.Sprintf("img_%d", imageIDSeq)
    imageStore[id] = imageEntry{img: img, fileSize: fileSize}
    return id
}

func retrieveImageEntry(id string) (imageEntry, bool) {
    imageStoreMu.RLock()          // shared read lock
    defer imageStoreMu.RUnlock()
    entry, ok := imageStore[id]
    return entry, ok
}
```

Go's built-in `map` is **not safe for concurrent use**. If two HTTP requests arrive simultaneously, one storing and one reading, you get a race condition. `sync.RWMutex` solves this:
- `Lock()` / `Unlock()` — exclusive access (writes)
- `RLock()` / `RUnlock()` — shared access (reads, multiple readers simultaneously)

### Error Handling

Go does not have exceptions. Errors are return values:

```go
func NewEncoder(options CompressionOptions) (*Encoder, error) {
    if options.Quality < 1 || options.Quality > 100 {
        return nil, fmt.Errorf("quality must be between 1 and 100, got %d", options.Quality)
    }
    return &Encoder{...}, nil
}
```

Callers must check the error:

```go
encoder, err := dct.NewEncoder(options)
if err != nil {
    return nil, fmt.Errorf("failed to create encoder: %w", err)
}
```

`%w` in `fmt.Errorf` **wraps** the error, preserving the original while adding context. `errors.Is()` and `errors.As()` can unwrap these chains later.

> **Real-World Analogy**: Go's error handling is like checking receipts at every step of a supply chain. Instead of one big try/catch at the end, each step verifies its own result and passes failures up explicitly. Nothing gets silently swallowed.

### `defer` — Cleanup Guaranteed

```go
file, err := os.Open(filePath)
if err != nil {
    return nil, err
}
defer file.Close()  // runs when the surrounding function returns
```

`defer` schedules a function call to run when the current function exits — whether normally, via `return`, or via `panic`. It is the Go idiom for resource cleanup: open → defer close → use.

---

## Array vs Slice vs Map

These three data structures appear constantly in DCTPress.

### Arrays — Fixed-Size, Stack-Allocated

```go
var block [8][8]float64          // 8×8 array of float64
var precomputedCosines [8][8]float64
var zigzag [64]int
```

Arrays have **fixed size known at compile time**. They are value types — assigning `b := a` copies every element. Arrays in Go are passed to functions by value (copying), unless you pass a pointer. The `[8][8]float64` block type is used throughout the DCT code because:
1. Size is known at compile time (always 8×8)
2. Stack allocation avoids GC pressure in the hot path
3. Fixed size enables type safety — `[7][8]float64` is a different type from `[8][8]float64`

### Slices — Dynamic-Size, Heap-Allocated

```go
plane := make([][]float64, height)
coefficients := make([]int, 0, totalBlocks*10)
```

A slice is a view into an underlying array: `(pointer, length, capacity)`. `make([]int, 0, n)` allocates an array of capacity n but initially length 0 — you can `append` up to n elements before reallocation.

```go
buf = append(buf, 'D', 'C', 'T', 0x02)  // append multiple elements
buf = append(buf, yEncoded...)           // append a slice (... unpacks it)
```

### Maps — Key-Value Stores

```go
frequencies := make(map[int]int, 512)  // pre-size hint for 512 entries
frequencies[coeff]++                   // increment count for coefficient
```

Maps in Go are hash tables. The `512` pre-size hint in `countFrequencies` avoids repeated rehashing as the map grows, since DCT coefficient streams have roughly 200-400 unique values.

---

## The `image` Standard Library

Go's `image` package is central to the entire project.

```go
// The universal image interface
type Image interface {
    ColorModel() color.Model
    Bounds() Rectangle
    At(x, y int) color.Color
}
```

`img.At(x, y).RGBA()` returns four `uint32` values in range [0, 65535]. The encoder shifts them by 8 bits to get [0, 255]:

```go
r, g, b, _ := img.At(x, y).RGBA()
rf := float64(r >> 8)  // r>>8 converts 16-bit to 8-bit
```

`image.NewRGBA(rect)` creates a mutable image backed by a flat byte slice (`pix`). The decoder uses this to build the output image pixel by pixel.

---

## Go's `flag` Package — CLI Arguments

```go
// cmd/server/main.go
port := flag.Int("port", envInt("SERVER_PORT", 8080), "HTTP server port")
flag.Parse()
```

`flag.Int` registers a command-line flag `--port`. The second argument is the default value. After `flag.Parse()`, `*port` holds the actual value (dereference because `flag.Int` returns a pointer).

`envInt` is a helper that first checks the `SERVER_PORT` environment variable, falling back to 8080. This supports both `./server --port 8081` and `SERVER_PORT=8081 ./server`.

---

## Summary

| Go concept | Where used in DCTPress | Why |
|-----------|----------------------|-----|
| `package` + `import` | Every file | Code organization |
| `init()` | `transform.go` | Precompute cosines once |
| `interface` | `image.Image` | Polymorphic image handling |
| Goroutines + channels | `encoder.go` | Parallel block processing |
| `sync.RWMutex` | `handlers.go` | Safe concurrent map access |
| Error return values | Everywhere | Explicit failure handling |
| `defer` | File I/O, mutexes | Guaranteed cleanup |
| Fixed arrays `[8][8]float64` | DCT loops | Stack allocation, type safety |
| `iota` + typed int | `ChromaSubsampling` | Enumeration |

---

*Next: [Phase 02 — Image Fundamentals & Color Spaces](02_image_fundamentals_and_color_spaces.md)*
