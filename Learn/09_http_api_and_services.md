# Phase 09 — HTTP API & Service Layer

> *The codec algorithms are pure functions. The HTTP API and service layer are the connective tissue that exposes them to the world — handling requests, managing state, and coordinating multiple components.*

---

## Architecture Layers

DCTPress uses a clean layered architecture:

```
Browser / curl
     │  HTTP
     ▼
handlers/         ← Parse HTTP, validate, call services, write JSON
     │  function call
     ▼
services/         ← Orchestrate: call codec + metrics + Python
     │  function call
     ▼
core/             ← Pure algorithms: DCT, Huffman, PSNR, SSIM
```

Each layer has a single responsibility. The HTTP layer knows nothing about DCT math; the core knows nothing about HTTP.

---

## Entry Point — `cmd/server/main.go`

```go
func main() {
    port := flag.Int("port", envInt("SERVER_PORT", 8080), "HTTP server port")
    flag.Parse()

    mux := http.NewServeMux()

    // Infrastructure
    mux.HandleFunc("/health", handlers.HealthHandler)

    // API routes
    mux.HandleFunc("/api/upload",    handlers.UploadHandler)
    mux.HandleFunc("/api/compress",  handlers.CompressHandler)
    mux.HandleFunc("/api/benchmark", handlers.BenchmarkHandler)
    mux.HandleFunc("/api/formats",   handlers.FormatsHandler)
    mux.HandleFunc("/api/download/", handlers.DownloadHandler)
    mux.HandleFunc("/api/export",    handlers.ExportHandler)

    // Serve static web UI
    webDir := "./web"
    if _, err := os.Stat(webDir); err == nil {
        mux.Handle("/", http.FileServer(http.Dir(webDir)))
    }

    address := fmt.Sprintf(":%d", *port)
    log.Printf("ImageCompressor-DCT server starting on http://localhost%s", address)
    if err := http.ListenAndServe(address, mux); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}
```

**`http.NewServeMux()`** — creates a request router. Unlike other frameworks, Go's stdlib `ServeMux` has simple path matching. `/api/download/` (with trailing slash) matches all paths starting with `/api/download/`.

**`http.FileServer(http.Dir(webDir))`** — serves static files from the `web/` directory. Any request that doesn't match a registered API route falls through to the file server, which serves `index.html`, `app.js`, `styles.css`.

**`os.Stat(webDir)`** — checks if the `web/` directory exists. If not (e.g., in a test environment), the file server is not registered and all requests go to the API. The compiled binary can run without the web assets.

**`envInt`** — helper that reads from environment variable first, then falls back to a default:

```go
func envInt(key string, def int) int {
    if v := os.Getenv(key); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            return n
        }
    }
    return def
}
```

This makes the server configurable via environment variable (`SERVER_PORT=8081`) or CLI flag (`--port 8081`) for Docker/container deployments.

---

## In-Memory State — handlers.go

The server is **stateless per-request** but maintains **in-memory state** between requests. Three maps store session data:

### Image Store

```go
type imageEntry struct {
    img      image.Image
    fileSize int  // original uploaded file size in bytes
}

var (
    imageStore   = make(map[string]imageEntry)
    imageStoreMu sync.RWMutex
    imageIDSeq   int
)
```

`imageStore` maps session IDs (`"img_1"`, `"img_2"`, ...) to uploaded images. Images remain in memory until the server restarts — there is no eviction. This is acceptable for a single-user demo tool; a production service would need an LRU cache or external storage.

`imageIDSeq` is a monotonically increasing counter. It is protected by `imageStoreMu` because HTTP requests can arrive concurrently.

### Download Store

```go
type downloadEntry struct {
    data        []byte
    contentType string
}

var (
    downloadStore   = make(map[string]downloadEntry)
    downloadStoreMu sync.RWMutex
    downloadIDSeq   int
)
```

Stores compressed binary blobs (`.dct` data) and preview images (JPEG bytes for browser display). Download IDs are used by the `/api/download/` and `/api/export` endpoints.

### Decoded Image Store

```go
type decodedEntry struct {
    img     image.Image
    quality int
}

var (
    decodedStore   = make(map[string]decodedEntry)
    decodedStoreMu sync.RWMutex
)
```

Stores decoded images (the round-trip output of compress+decode) for the export endpoint. The quality is stored to re-encode as JPEG at the same quality for the export.

---

## `UploadHandler` — POST /api/upload

```go
func UploadHandler(writer http.ResponseWriter, request *http.Request) {
    if request.Method != http.MethodPost {
        writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    if err := request.ParseMultipartForm(maxUploadBytes); err != nil {
        writeError(writer, http.StatusBadRequest, "failed to parse multipart form")
        return
    }

    file, header, err := request.FormFile("image")
    // ...
    defer file.Close()

    if header.Size > maxUploadBytes {
        writeError(writer, http.StatusRequestEntityTooLarge, ...)
        return
    }

    img, format, err := image.Decode(file)
    // ...

    imageID := storeImage(img, int(header.Size))
    bounds := img.Bounds()

    writeJSON(writer, http.StatusOK, models.ImageMetadata{
        ID:        imageID,
        Format:    format,
        Width:     bounds.Dx(),
        Height:    bounds.Dy(),
        SizeBytes: int(header.Size),
        ColorMode: "RGB",
    })
}
```

**`ParseMultipartForm(maxUploadBytes)`** — parses the HTTP multipart form, buffering up to `maxUploadBytes` (10 MB) in memory. Files larger than this are spilled to disk.

**`FormFile("image")`** — retrieves the file uploaded under the form field name `"image"`. Returns the file reader, file header (name, size, MIME type), and error.

**`image.Decode(file)`** — decodes any registered image format. The format string ("jpeg", "png", "webp", "bmp", "tiff") is returned and stored in the metadata response.

**`storeImage(img, int(header.Size))`** — stores the decoded `image.Image` in the server's in-memory map and returns a session ID.

**Why store the file size separately?** The `fileSize` is the size of the uploaded file (possibly already a JPEG at some quality). `CompressionResult.OriginalSize` is the raw RGB bytes (`width × height × 3`). Both are useful: the file ratio tells you how much you saved vs the user's file; the RGB ratio tells you how much you saved vs raw data. The API response includes both.

---

## `CompressHandler` — POST /api/compress

```go
func CompressHandler(writer http.ResponseWriter, request *http.Request) {
    var requestBody struct {
        ImageID           string `json:"image_id"`
        Quality           int    `json:"quality"`
        ChromaSubsampling string `json:"chroma_subsampling"`
    }

    if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
        writeError(writer, http.StatusBadRequest, "invalid JSON body")
        return
    }
    
    // ...validate quality...
    
    entry, ok := retrieveImageEntry(requestBody.ImageID)
    if !ok {
        writeError(writer, http.StatusNotFound, ...)
        return
    }

    subsampling := parseChromaSubsampling(requestBody.ChromaSubsampling)

    compressionService := services.NewCompressionService()
    response, err := compressionService.Compress(services.CompressRequest{
        Image:             entry.img,
        Quality:           requestBody.Quality,
        ChromaSubsampling: subsampling,
    })
    // ...

    downloadID := storeDownload(response.Result.Data, "application/octet-stream")
    storeDecodedImage(downloadID, response.DecodedImage, requestBody.Quality)

    // Encode decompressed image as JPEG for browser preview
    var previewBuf bytes.Buffer
    jpeg.Encode(&previewBuf, response.DecodedImage, &jpeg.Options{Quality: 90})
    previewID := storeDownload(previewBuf.Bytes(), "image/jpeg")

    writeJSON(writer, http.StatusOK, map[string]interface{}{
        "compressed_image_url": fmt.Sprintf("/api/download/%s", downloadID),
        "export_id":            downloadID,
        "compression_ratio":    response.Result.CompressionRatio,
        "file_ratio":           float64(entry.fileSize) / float64(response.Result.CompressedSize),
        "psnr":                 response.PSNR,
        "ssim":                 response.SSIM,
        "encoding_time_ms":     response.EncodingTime.Milliseconds(),
        ...
    })
}
```

**`json.NewDecoder(request.Body).Decode(&requestBody)`** — streaming JSON decode from the request body. More memory-efficient than `ioutil.ReadAll` + `json.Unmarshal` for large bodies (though request bodies here are tiny).

**Struct tags `json:"image_id"`** — map JSON field names (snake_case) to Go struct fields (PascalCase). Without tags, Go uses the exact field name, which is PascalCase.

**`parseChromaSubsampling`**:

```go
func parseChromaSubsampling(value string) dct.ChromaSubsampling {
    switch value {
    case "4:2:2": return dct.Subsampling422
    case "4:4:4": return dct.Subsampling444
    default:      return dct.Subsampling420  // default to 4:2:0
    }
}
```

Converts the human-readable string from the API request to the typed enum value. Unknown values default to 4:2:0.

**Preview JPEG encoding**: The `.dct` format cannot be displayed in a browser. After decompression, the decoded image is re-encoded as JPEG at quality=90 and stored for browser display via the `preview_url` field. This is the image shown in the "decompressed" panel of the web UI.

**Why two stores for one compression?** The download store holds the raw `.dct` bytes. The decoded store holds the `image.Image` + quality for the export endpoint (which re-encodes as JPEG or PNG on demand). Separating them avoids re-decoding the `.dct` bytes for every export request.

---

## `BenchmarkHandler` — POST /api/benchmark

```go
func BenchmarkHandler(writer http.ResponseWriter, request *http.Request) {
    // ...parse imageID and quality...
    benchmarkService := services.NewBenchmarkService()
    results := benchmarkService.RunAll(entry.img, quality)
    writeJSON(writer, http.StatusOK, map[string]interface{}{
        "quality": quality,
        "results": results,
    })
}
```

Delegates entirely to `BenchmarkService.RunAll()`. The handler's job is just parsing the request and serializing the response.

---

## `ExportHandler` — GET /api/export

```go
func ExportHandler(writer http.ResponseWriter, request *http.Request) {
    id := request.URL.Query().Get("id")
    format := request.URL.Query().Get("format")
    
    switch format {
    case "dct":
        entry, ok := retrieveDownload(id)
        // serve raw .dct bytes
        writer.Header().Set("Content-Type", "application/octet-stream")
        writer.Header().Set("Content-Disposition", `attachment; filename="compressed_dl_1.dct"`)
        writer.Write(entry.data)
        
    case "jpeg", "png", "":
        de, ok := retrieveDecodedImage(id)
        var buf bytes.Buffer
        if format == "png" {
            png.Encode(&buf, de.img)
            writer.Header().Set("Content-Type", "image/png")
        } else {
            jpeg.Encode(&buf, de.img, &jpeg.Options{Quality: de.quality})
            writer.Header().Set("Content-Type", "image/jpeg")
        }
        writer.Write(buf.Bytes())
    }
}
```

Three output formats:
1. **DCT**: raw `.dct` binary (smallest, DCTPress-specific format)
2. **JPEG**: decoded image re-encoded as JPEG (browser-viewable, standard)
3. **PNG**: decoded image as lossless PNG (larger but lossless from decoded state)

`Content-Disposition: attachment` tells browsers to download the file rather than display it.

---

## `CompressionService` — services/compression_service.go

```go
func (service *CompressionService) Compress(request CompressRequest) (*CompressResponse, error) {
    encoder, err := dct.NewEncoder(dct.CompressionOptions{
        Quality:           request.Quality,
        ChromaSubsampling: request.ChromaSubsampling,
    })
    
    encodeStart := time.Now()
    result, err := encoder.Encode(request.Image)
    encodingTime := time.Since(encodeStart)
    
    decoder := dct.NewDecoder()
    decodedImage, err := decoder.Decode(result.Data)
    
    psnrValue, _ := metrics.CalculatePSNR(request.Image, decodedImage)
    ssimValue, _ := metrics.CalculateSSIM(request.Image, decodedImage)
    
    return &CompressResponse{
        Result:       result,
        PSNR:         psnrValue,
        SSIM:         ssimValue,
        EncodingTime: encodingTime,
        DecodedImage: decodedImage,
    }, nil
}
```

`time.Now()` / `time.Since()` — measures elapsed wall-clock time. `encodingTime` includes only the `Encode()` call, not the subsequent decode and metric calculation. The "179 ms" in benchmarks includes the full pipeline.

`CompressGoJPEG` in the same file provides the Go stdlib JPEG baseline for the benchmark:

```go
func CompressGoJPEG(img image.Image, quality int) (models.BenchmarkResult, error) {
    var buf bytes.Buffer
    jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
    decodedImage, _ := jpeg.Decode(bytes.NewReader(buf.Bytes()))
    // calculate PSNR + SSIM vs original
    // return BenchmarkResult
}
```

Same pattern: encode → decode → measure. Makes the comparison fair — all codecs measure round-trip quality.

---

## `models.go` — Shared Domain Types

```go
type CompressionResult struct {
    Data             []byte    // the .dct binary
    OriginalSize     int       // width × height × 3
    CompressedSize   int       // len(Data)
    CompressionRatio float64   // OriginalSize / CompressedSize
    Width, Height    int
    Quality          int
}

type BenchmarkResult struct {
    Method           string
    Language         string
    CompressedSize   int
    CompressionRatio float64
    EncodingTime     time.Duration
    DecodingTime     time.Duration
    PSNR             float64
    SSIM             float64
    MemoryMB         float64
}

type ImageMetadata struct {
    ID        string `json:"id"`
    Format    string `json:"format"`
    Width     int    `json:"width"`
    Height    int    `json:"height"`
    SizeBytes int    `json:"size_bytes"`
    ColorMode string `json:"color_mode"`
}
```

`models.go` is intentionally simple — it defines the **data shapes** that flow between layers. No logic lives here. This separation means the core algorithm layer (`core/`) can be developed and tested independently of the HTTP layer.

**JSON struct tags**: `json:"id"` makes the serialized field name `"id"` in the HTTP response, not `"ID"`. Go's JSON encoder respects these tags.

---

## Helper Functions

```go
func writeJSON(writer http.ResponseWriter, statusCode int, payload interface{}) {
    writer.Header().Set("Content-Type", "application/json")
    writer.WriteHeader(statusCode)
    json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, statusCode int, message string) {
    writeJSON(writer, statusCode, map[string]string{"error": message})
}
```

`json.NewEncoder(writer).Encode(payload)` — streams JSON directly to the HTTP response writer, avoiding a `bytes.Buffer` intermediate. Efficient for large responses.

The pattern `map[string]string{"error": message}` produces a consistent error response format: `{"error": "description"}`.

---

## The Complete Request Lifecycle

```
1. Browser: POST /api/upload (multipart/form-data, image file)
   → UploadHandler
     → ParseMultipartForm()
     → image.Decode()
     → storeImage() → "img_1"
   → Response: {"id": "img_1", "width": 1920, ...}

2. Browser: POST /api/compress (JSON)
   → CompressHandler
     → json.Decode() → {image_id: "img_1", quality: 75}
     → retrieveImageEntry("img_1") → image.Image
     → CompressionService.Compress()
       → dct.NewEncoder()
       → encoder.Encode() → CompressionResult
       → decoder.Decode() → decoded image.Image
       → CalculatePSNR(), CalculateSSIM()
     → storeDownload(result.Data) → "dl_1"
     → storeDecodedImage("dl_1", decodedImage, 75)
     → jpeg.Encode(decodedImage) → storeDownload() → "dl_2" (preview)
   → Response: {"export_id": "dl_1", "preview_url": "/api/download/dl_2", "psnr": 33.77, ...}

3. Browser displays preview: GET /api/download/dl_2
   → DownloadHandler
     → retrieveDownload("dl_2") → {data: []byte (JPEG), contentType: "image/jpeg"}
   → Response: raw JPEG bytes

4. User clicks "Download DCT": GET /api/export?id=dl_1&format=dct
   → ExportHandler
     → retrieveDownload("dl_1") → {data: []byte (.dct)}
   → Response: .dct bytes, Content-Disposition: attachment
```

---

## Health Endpoint — `health.go`

```go
func HealthHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    pythonOK := exec.CommandContext(ctx, "python3", "--version").Run() == nil

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "status":    "healthy",
        "timestamp": time.Now().UTC().Format(time.RFC3339),
        "python":    pythonOK,
        "version":   "1.0.0",
    })
}
```

Checks if Python is available (within 3-second timeout) and reports it in the health response. Used by Docker's `HEALTHCHECK` and load balancer probes.

`context.WithTimeout` — creates a context that automatically cancels after 3 seconds. `exec.CommandContext` respects this context; if Python doesn't respond in 3 seconds, the command is killed and `err != nil`.

`time.RFC3339` is Go's constant for the RFC 3339 timestamp format: `"2006-01-02T15:04:05Z07:00"`. The specific values (2006, 01, 02, 15, 04, 05) are Go's reference time — they are mnemonic (month=01, day=02, hour=15=3PM, minute=04, second=05, year=2006).

---

*Next: [Phase 10 — Python Bridge & Benchmarking](10_python_bridge_and_benchmarking.md)*
