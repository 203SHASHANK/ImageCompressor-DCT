# **Image Compression Engine - Complete Implementation Specification**
// keep updating /home/shashanks/sike/ImageCompressor-DCT/context.md   to track the implementation at each levels
## **PROJECT METADATA**
```yaml
Project Name: ImageCompressor-DCT
Author: Shashank S 
Language: Go 1.21+
Purpose: Production-grade multi-format image compression with DCT algorithm benchmarking
Resume Impact: Demonstrates algorithms, performance optimization, clean architecture, benchmarking
Unique Selling Points:
  - Custom DCT-based compression beating standard libraries by 20-30%
  - Multi-format support (PNG, JPEG, WebP, BMP, TIFF)
  - Real-time quality metrics (PSNR, SSIM)
  - Cross-language benchmark (Go vs Python libraries)
  - Clean single-page web UI with live preview
  - Production-ready Go architecture
```

---

## **TABLE OF CONTENTS**
1. [Project Overview](#1-project-overview)
2. [Technical Requirements](#2-technical-requirements)
3. [System Architecture](#3-system-architecture)
4. [Directory Structure](#4-directory-structure)
5. [Core Components Specification](#5-core-components-specification)
6. [Implementation Roadmap](#6-implementation-roadmap)
7. [API Specifications](#7-api-specifications)
8. [UI/UX Requirements](#8-uiux-requirements)
9. [Benchmarking Requirements](#9-benchmarking-requirements)
10. [Code Quality Standards](#10-code-quality-standards)
11. [Testing Strategy](#11-testing-strategy)
12. [Performance Targets](#12-performance-targets)
13. [Deployment Guide](#13-deployment-guide)
14. [Learning Outcomes](#14-learning-outcomes)

---
## **1. PROJECT OVERVIEW**

### **1.1 Problem Statement**
Standard image compression libraries (JPEG, PNG, WebP) are black boxes with no visibility into compression trade-offs. Developers need a tool to:
- Understand DCT-based compression algorithms
- Compare custom implementations against industry standards
- Visualize quality vs size trade-offs in real-time
- Benchmark across multiple languages and libraries

### **1.2 Solution**
Build a complete image compression system featuring:
- **Custom DCT Algorithm**: JPEG-like compression with tunable parameters
- **Multi-Format Support**: Input (PNG/JPEG/WebP/BMP/TIFF) → Custom .dct format → Output (JPEG/WebP/PNG)
- **Live Benchmarking**: Real-time comparison against Go stdlib, WebP, Python PIL/OpenCV
- **Interactive UI**: Single-page dashboard with drag-drop, quality slider, metrics overlay
- **Educational Value**: Visualize DCT coefficients, quantization tables, compression artifacts

### **1.3 Success Criteria**
```yaml
Performance:
  - Compression: 5MB PNG → <500KB custom format (10x reduction)
  - Speed: Encode 1920x1080 image in <500ms
  - Quality: PSNR >30dB at 50% quality setting
  - Memory: <100MB peak usage for 4K images

Functionality:
  - Support 5+ input formats
  - 3+ compression quality presets (Low/Medium/High)
  - Chroma subsampling options (4:2:0, 4:2:2, 4:4:4)
  - Batch processing (compress folder of images)
  - Cross-platform (Linux, macOS, Windows)

Benchmarking:
  - Compare against 4+ libraries (Go JPEG, WebP, Python PIL, OpenCV)
  - Generate CSV reports with 6+ metrics
  - Visual diff heatmaps

Code Quality:
  - 100% test coverage for core algorithms
  - Zero linter warnings
  - Comprehensive inline documentation
  - Clean architecture (repository pattern)
```

---

## **2. TECHNICAL REQUIREMENTS**

### **2.1 Hard Requirements (MANDATORY)**

#### **2.1.1 Language & Runtime**
```yaml
Go Version: 1.21 or higher
Reason: Generics, improved performance, better error handling

Required Go Modules:
  - image: Image decoding/encoding
  - image/jpeg: JPEG support
  - image/png: PNG support
  - net/http: Web server
  - encoding/json: API responses
  - testing: Unit tests
  - math: DCT calculations
  
Third-Party Libraries (APPROVED ONLY):
  - github.com/chai2010/webp: WebP support
  - github.com/disintegration/imaging: Image resizing
  - github.com/gorilla/mux: HTTP routing (optional, stdlib is fine)
  - github.com/stretchr/testify: Testing assertions
```

#### **2.1.2 Python Benchmark Integration**
```yaml
Python Version: 3.9+
Required Libraries:
  - Pillow (PIL): JPEG/PNG compression
  - opencv-python: Advanced compression
  - scikit-image: PSNR/SSIM calculation
  - numpy: Image array operations

Integration Method:
  - Go calls Python scripts via exec.Command
  - Pass image paths as arguments
  - Parse JSON output from Python
  - Never mix Go and Python in same process
```

#### **2.1.3 Frontend Requirements**
```yaml
Technology: Pure HTML5 + CSS3 + Vanilla JavaScript
NO Frameworks: No React, Vue, Angular (keep it simple)

Features:
  - Drag-and-drop file upload
  - Real-time quality slider (1-100)
  - Side-by-side image comparison
  - Live metrics display (file size, PSNR, time)
  - Responsive design (mobile-friendly)
  - Dark mode support

Browser Support:
  - Chrome 90+
  - Firefox 88+
  - Safari 14+
  - Edge 90+
```

### **2.2 Soft Requirements (NICE-TO-HAVE)**

```yaml
Optional Enhancements:
  - GPU acceleration (OpenCL/CUDA) for DCT
  - Progressive encoding (like progressive JPEG)
  - Arithmetic coding (better than Huffman)
  - Video frame compression
  - Docker containerization
  - CI/CD pipeline (GitHub Actions)
```

---

## **3. SYSTEM ARCHITECTURE**

### **3.1 High-Level Architecture**

```
┌─────────────────────────────────────────────────────────────────┐
│                     ImageCompressor-DCT System                   │
└─────────────────────────────────────────────────────────────────┘

┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│   Frontend   │ ◄────── │   HTTP API   │ ◄────── │   Core       │
│   (Web UI)   │  JSON   │   (Handlers) │  Domain │   Engine     │
│              │         │              │  Models │              │
└──────────────┘         └──────────────┘         └──────┬───────┘
                                                          │
                         ┌────────────────────────────────┼────────┐
                         │                                │        │
                    ┌────▼─────┐                   ┌──────▼──────┐ │
                    │   DCT    │                   │   Format    │ │
                    │ Encoder  │                   │  Handlers   │ │
                    └────┬─────┘                   └──────┬──────┘ │
                         │                                │        │
                    ┌────▼──────┐                  ┌──────▼──────┐ │
                    │Quantizer  │                  │  I/O Utils  │ │
                    └────┬──────┘                  └──────┬──────┘ │
                         │                                │        │
                    ┌────▼──────┐                  ┌──────▼──────┐ │
                    │  Huffman  │                  │ Benchmark   │◄┘
                    │  Encoder  │                  │   Runner    │
                    └───────────┘                  └──────┬──────┘
                                                          │
                                                   ┌──────▼──────┐
                                                   │   Python    │
                                                   │   Bridge    │
                                                   └─────────────┘
```

### **3.2 Layered Architecture (Clean Architecture)**

```
┌─────────────────────────────────────────────────────────┐
│  Layer 1: Presentation (handlers/)                       │
│  - HTTP handlers                                        │
│  - Request validation                                   │
│  - Response formatting                                  │
│  - Static file serving                                  │
└────────────────────┬────────────────────────────────────┘
                     │ (uses)
┌────────────────────▼────────────────────────────────────┐
│  Layer 2: Application (services/)                       │
│  - Business logic orchestration                         │
│  - Compression workflows                                │
│  - Benchmark coordination                               │
│  - Error handling & logging                             │
└────────────────────┬────────────────────────────────────┘
                     │ (uses)
┌────────────────────▼────────────────────────────────────┐
│  Layer 3: Domain (core/)                                │
│  - DCT algorithm implementation                         │
│  - Quantization logic                                   │
│  - Huffman encoding                                     │
│  - Quality metrics (PSNR, SSIM)                         │
│  - Pure functions, no I/O                               │
└────────────────────┬────────────────────────────────────┘
                     │ (uses)
┌────────────────────▼────────────────────────────────────┐
│  Layer 4: Infrastructure (internal/)                    │
│  - File I/O operations                                  │
│  - Image format parsers                                 │
│  - Python subprocess execution                          │
│  - Configuration loading                                │
└─────────────────────────────────────────────────────────┘
```

### **3.3 Data Flow Diagram**

```
USER UPLOAD (PNG/JPEG/WebP)
        │
        ▼
┌───────────────────┐
│  HTTP Handler     │ (handlers/upload.go)
│  - Validate file  │
│  - Parse quality  │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Compression       │ (services/compressor.go)
│ Service           │
│ - Load image      │
│ - Route to algo   │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ DCT Encoder       │ (core/dct/encoder.go)
│ - Split 8x8       │
│ - Apply DCT       │
│ - Quantize        │
│ - Huffman encode  │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Output Writer     │ (internal/io/writer.go)
│ - Write .dct file │
│ - Export JPEG     │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Benchmark Runner  │ (services/benchmark.go)
│ - Run Go JPEG     │
│ - Run Python PIL  │
│ - Calculate PSNR  │
│ - Generate report │
└────────┬──────────┘
         │
         ▼
    JSON RESPONSE
    (metrics + compressed image)
```

---

## **4. DIRECTORY STRUCTURE**

### **4.1 Complete Project Layout**

```
imagecompressor-dct/
│
├── cmd/
│   └── server/
│       └── main.go                    # Entry point (HTTP server)
│
├── internal/
│   ├── handlers/                      # HTTP request handlers
│   │   ├── upload_handler.go          # File upload endpoint
│   │   ├── compress_handler.go        # Compression endpoint
│   │   ├── benchmark_handler.go       # Benchmark endpoint
│   │   └── static_handler.go          # Serve frontend
│   │
│   ├── services/                      # Business logic
│   │   ├── compression_service.go     # Orchestrate compression
│   │   ├── benchmark_service.go       # Run benchmarks
│   │   └── validation_service.go      # Input validation
│   │
│   ├── core/                          # Core algorithms (pure logic)
│   │   ├── dct/
│   │   │   ├── encoder.go             # DCT compression
│   │   │   ├── decoder.go             # DCT decompression
│   │   │   ├── transform.go           # 2D DCT math
│   │   │   └── quantization.go        # Quantization tables
│   │   │
│   │   ├── huffman/
│   │   │   ├── encoder.go             # Huffman tree builder
│   │   │   └── decoder.go             # Huffman decoder
│   │   │
│   │   └── metrics/
│   │       ├── psnr.go                # PSNR calculation
│   │       ├── ssim.go                # SSIM calculation
│   │       └── mse.go                 # Mean Squared Error
│   │
│   ├── formats/                       # Image format handlers
│   │   ├── png_handler.go             # PNG read/write
│   │   ├── jpeg_handler.go            # JPEG read/write
│   │   ├── webp_handler.go            # WebP read/write
│   │   ├── bmp_handler.go             # BMP read/write
│   │   └── dct_format.go              # Custom .dct format
│   │
│   ├── io/                            # File I/O utilities
│   │   ├── image_loader.go            # Load images
│   │   ├── image_writer.go            # Write images
│   │   └── temp_file.go               # Temp file management
│   │
│   ├── python/                        # Python integration
│   │   ├── bridge.go                  # Execute Python scripts
│   │   └── scripts/
│   │       ├── compress_pil.py        # PIL compression
│   │       ├── compress_opencv.py     # OpenCV compression
│   │       └── calculate_metrics.py   # Python-based PSNR/SSIM
│   │
│   ├── models/                        # Domain models
│   │   ├── image.go                   # Image metadata
│   │   ├── compression_result.go      # Compression output
│   │   └── benchmark_result.go        # Benchmark report
│   │
│   └── config/
│       └── config.go                  # Configuration loader
│
├── web/                               # Frontend files
│   ├── index.html                     # Single-page UI
│   ├── styles.css                     # Styles (dark mode)
│   ├── app.js                         # JavaScript logic
│   └── assets/
│       └── favicon.ico
│
├── test/
│   ├── testdata/                      # Test images
│   │   ├── sample.png
│   │   ├── sample.jpg
│   │   └── sample.webp
│   │
│   ├── unit/                          # Unit tests
│   │   ├── dct_test.go
│   │   ├── huffman_test.go
│   │   └── metrics_test.go
│   │
│   ├── integration/                   # Integration tests
│   │   └── compression_flow_test.go
│   │
│   └── benchmarks/                    # Go benchmarks
│       └── compression_bench_test.go
│
├── docs/
│   ├── ARCHITECTURE.md                # This document
│   ├── API.md                         # API documentation
│   └── ALGORITHM.md                   # DCT algorithm explained
│
├── scripts/
│   ├── setup.sh                       # Setup script
│   └── run_benchmarks.sh              # Benchmark runner
│
├── .gitignore
├── go.mod
├── go.sum
├── Makefile                           # Build automation
└── README.md                          # Project overview
```

### **4.2 File Naming Conventions**

```yaml
Go Files:
  - snake_case: compression_service.go ✅
  - NOT camelCase: compressionService.go ❌
  
Test Files:
  - Suffix _test.go: dct_test.go ✅
  
Directories:
  - Lowercase, singular: handler/ ❌ handlers/ ✅
  
Constants/Variables:
  - Exported: CompressionQualityHigh (PascalCase)
  - Internal: compressionQualityHigh (camelCase)
  - Private: _internalBuffer (underscore prefix for truly private)
```

---

## **5. CORE COMPONENTS SPECIFICATION**

### **5.1 DCT Encoder (core/dct/encoder.go)**

#### **5.1.1 Purpose**
Implement Discrete Cosine Transform-based image compression similar to JPEG baseline algorithm.

#### **5.1.2 Algorithm Steps**
```
1. Color Space Conversion: RGB → YCbCr
2. Chroma Subsampling: 4:4:4 / 4:2:2 / 4:2:0
3. Block Splitting: 8x8 pixel blocks
4. DCT Transform: Apply 2D DCT to each block
5. Quantization: Divide by quantization table
6. Zigzag Scan: Reorder coefficients
7. Huffman Encoding: Compress coefficient stream
8. Write to .dct format
```

#### **5.1.3 Function Signatures**

```go
package dct

import (
    "image"
    "imagecompressor-dct/internal/models"
)

// CompressionOptions defines tunable parameters
type CompressionOptions struct {
    Quality           int      // 1-100, higher = better quality
    ChromaSubsampling string   // "4:4:4", "4:2:2", "4:2:0"
    BlockSize         int      // 8, 16, or 32
    UseArithmeticCoding bool   // Use arithmetic instead of Huffman
}

// Encoder performs DCT-based compression
type Encoder struct {
    options CompressionOptions
    
    // Quantization tables (luminance and chrominance)
    luminanceQuantTable   [8][8]float64
    chrominanceQuantTable [8][8]float64
}

// NewEncoder creates a configured encoder
func NewEncoder(options CompressionOptions) *Encoder {
    encoder := &Encoder{options: options}
    encoder.initializeQuantizationTables()
    return encoder
}

// Encode compresses an image to byte stream
// Returns: compressed data, metadata, error
func (e *Encoder) Encode(img image.Image) (*models.CompressionResult, error) {
    // STEP 1: Convert RGB to YCbCr
    ycbcrImage := convertToYCbCr(img)
    
    // STEP 2: Apply chroma subsampling
    subsampledImage := e.applyChromaSubsampling(ycbcrImage)
    
    // STEP 3: Split into 8x8 blocks
    blocks := splitIntoBlocks(subsampledImage, e.options.BlockSize)
    
    // STEP 4: Apply DCT to each block
    dctBlocks := e.applyDCTToBlocks(blocks)
    
    // STEP 5: Quantize coefficients
    quantizedBlocks := e.quantizeBlocks(dctBlocks)
    
    // STEP 6: Zigzag scan and flatten
    coefficients := zigzagScan(quantizedBlocks)
    
    // STEP 7: Huffman encode
    compressedData := e.huffmanEncode(coefficients)
    
    // STEP 8: Build result
    result := &models.CompressionResult{
        Data:             compressedData,
        OriginalSize:     calculateImageSize(img),
        CompressedSize:   len(compressedData),
        CompressionRatio: float64(calculateImageSize(img)) / float64(len(compressedData)),
        Width:            img.Bounds().Dx(),
        Height:           img.Bounds().Dy(),
        Quality:          e.options.Quality,
    }
    
    return result, nil
}

// applyDCTToBlocks applies 2D DCT to each 8x8 block
func (e *Encoder) applyDCTToBlocks(blocks [][][]float64) [][][]float64 {
    dctBlocks := make([][][]float64, len(blocks))
    
    for i, block := range blocks {
        dctBlocks[i] = applyDCT2D(block)
    }
    
    return dctBlocks
}

// applyDCT2D performs 2D Discrete Cosine Transform
func applyDCT2D(block [][]float64) [][]float64 {
    blockSize := len(block)
    dctBlock := make([][]float64, blockSize)
    
    for u := 0; u < blockSize; u++ {
        dctBlock[u] = make([]float64, blockSize)
        for v := 0; v < blockSize; v++ {
            sum := 0.0
            
            for x := 0; x < blockSize; x++ {
                for y := 0; y < blockSize; y++ {
                    sum += block[x][y] *
                        math.Cos((2*float64(x)+1)*float64(u)*math.Pi/(2*float64(blockSize))) *
                        math.Cos((2*float64(y)+1)*float64(v)*math.Pi/(2*float64(blockSize)))
                }
            }
            
            alphaU := 1.0
            if u == 0 {
                alphaU = 1.0 / math.Sqrt(2)
            }
            alphaV := 1.0
            if v == 0 {
                alphaV = 1.0 / math.Sqrt(2)
            }
            
            dctBlock[u][v] = 0.25 * alphaU * alphaV * sum
        }
    }
    
    return dctBlock
}

// quantizeBlocks divides DCT coefficients by quantization table
func (e *Encoder) quantizeBlocks(dctBlocks [][][]float64) [][][]int {
    quantizedBlocks := make([][][]int, len(dctBlocks))
    
    for i, block := range dctBlocks {
        quantizedBlocks[i] = e.quantizeSingleBlock(block)
    }
    
    return quantizedBlocks
}

// quantizeSingleBlock quantizes one 8x8 block
func (e *Encoder) quantizeSingleBlock(block [][]float64) [][]int {
    blockSize := len(block)
    quantized := make([][]int, blockSize)
    
    for u := 0; u < blockSize; u++ {
        quantized[u] = make([]int, blockSize)
        for v := 0; v < blockSize; v++ {
            // Use luminance table (could use chrominance for Cb/Cr)
            quantized[u][v] = int(math.Round(block[u][v] / e.luminanceQuantTable[u][v]))
        }
    }
    
    return quantized
}

// initializeQuantizationTables creates quality-scaled tables
func (e *Encoder) initializeQuantizationTables() {
    // JPEG standard luminance table
    standardLuminanceTable := [8][8]float64{
        {16, 11, 10, 16, 24, 40, 51, 61},
        {12, 12, 14, 19, 26, 58, 60, 55},
        {14, 13, 16, 24, 40, 57, 69, 56},
        {14, 17, 22, 29, 51, 87, 80, 62},
        {18, 22, 37, 56, 68, 109, 103, 77},
        {24, 35, 55, 64, 81, 104, 113, 92},
        {49, 64, 78, 87, 103, 121, 120, 101},
        {72, 92, 95, 98, 112, 100, 103, 99},
    }
    
    // Scale table based on quality (1-100)
    scaleFactor := calculateQualityScaleFactor(e.options.Quality)
    
    for u := 0; u < 8; u++ {
        for v := 0; v < 8; v++ {
            e.luminanceQuantTable[u][v] = standardLuminanceTable[u][v] * scaleFactor
            // Prevent division by zero
            if e.luminanceQuantTable[u][v] < 1 {
                e.luminanceQuantTable[u][v] = 1
            }
        }
    }
    
    // Initialize chrominance table (similar process)
    // ... (omitted for brevity)
}

// calculateQualityScaleFactor maps quality (1-100) to scale factor
func calculateQualityScaleFactor(quality int) float64 {
    if quality < 50 {
        return 50.0 / float64(quality)
    }
    return 2.0 - float64(quality)/50.0
}

// zigzagScan reorders coefficients in zigzag pattern
func zigzagScan(blocks [][][]int) []int {
    // Zigzag order for 8x8 block
    zigzagOrder := []struct{ u, v int }{
        {0, 0}, {0, 1}, {1, 0}, {2, 0}, {1, 1}, {0, 2}, {0, 3}, {1, 2},
        {2, 1}, {3, 0}, {4, 0}, {3, 1}, {2, 2}, {1, 3}, {0, 4}, {0, 5},
        // ... (complete 64-element zigzag pattern)
    }
    
    coefficients := []int{}
    for _, block := range blocks {
        for _, pos := range zigzagOrder {
            coefficients = append(coefficients, block[pos.u][pos.v])
        }
    }
    
    return coefficients
}

// Helper functions (implement these)
func convertToYCbCr(img image.Image) *image.YCbCr { /* ... */ }
func (e *Encoder) applyChromaSubsampling(img *image.YCbCr) *image.YCbCr { /* ... */ }
func splitIntoBlocks(img *image.YCbCr, blockSize int) [][][]float64 { /* ... */ }
func calculateImageSize(img image.Image) int { /* ... */ }
func (e *Encoder) huffmanEncode(coefficients []int) []byte { /* ... */ }
```

#### **5.1.4 Implementation Requirements**

```yaml
MANDATORY:
  - All functions MUST have descriptive comments
  - Variable names MUST be human-readable (NO single letters except i, j for loops)
  - Error handling on EVERY operation that can fail
  - Unit tests with 100% code coverage
  - Benchmark tests for performance measurement

FORBIDDEN:
  - Global variables (use struct fields)
  - Magic numbers (define constants)
  - Panics (return errors instead)
  - External dependencies for core math (pure Go)

PERFORMANCE:
  - DCT calculation MUST complete in <50ms for 8x8 block
  - Memory allocation MUST be pre-allocated (no dynamic growth in hot paths)
  - Use math.Sqrt lookup table for repeated calculations
```

---

### **5.2 Huffman Encoder (core/huffman/encoder.go)**

#### **5.2.1 Purpose**
Lossless entropy encoding of quantized DCT coefficients.

#### **5.2.2 Algorithm**
```
1. Frequency Analysis: Count occurrence of each coefficient value
2. Build Huffman Tree: Create binary tree based on frequencies
3. Generate Codes: Assign bit patterns (short for frequent, long for rare)
4. Encode Data: Replace coefficients with Huffman codes
5. Write Header: Store tree structure for decoding
```

#### **5.2.3 Function Signatures**

```go
package huffman

// Node represents a Huffman tree node
type Node struct {
    Value       int   // Coefficient value (-2047 to 2047 for JPEG)
    Frequency   int   // Occurrence count
    LeftChild   *Node
    RightChild  *Node
}

// CodeTable maps coefficient values to bit patterns
type CodeTable map[int]string // e.g., {0: "00", 1: "01", -1: "100"}

// Encoder builds and uses Huffman tree
type Encoder struct {
    rootNode  *Node
    codeTable CodeTable
}

// NewEncoder creates encoder from coefficient data
func NewEncoder(coefficients []int) *Encoder {
    frequencies := calculateFrequencies(coefficients)
    tree := buildHuffmanTree(frequencies)
    codeTable := generateCodeTable(tree)
    
    return &Encoder{
        rootNode:  tree,
        codeTable: codeTable,
    }
}

// Encode compresses coefficients to bit stream
func (e *Encoder) Encode(coefficients []int) ([]byte, error) {
    bitStream := ""
    
    for _, coefficient := range coefficients {
        code, exists := e.codeTable[coefficient]
        if !exists {
            return nil, fmt.Errorf("coefficient %d not in code table", coefficient)
        }
        bitStream += code
    }
    
    // Convert bit string to bytes
    return bitStreamToBytes(bitStream), nil
}

// SerializeTree writes tree structure to bytes (for decoder)
func (e *Encoder) SerializeTree() []byte {
    // Serialize tree in pre-order traversal
    // Format: [leaf_flag(1bit)][value(11bits if leaf)] repeat
    // This is needed for decoder to reconstruct tree
}

// Helper functions
func calculateFrequencies(coefficients []int) map[int]int {
    frequencies := make(map[int]int)
    for _, coeff := range coefficients {
        frequencies[coeff]++
    }
    return frequencies
}

func buildHuffmanTree(frequencies map[int]int) *Node {
    // Priority queue implementation
    // Build tree bottom-up
}

func generateCodeTable(root *Node) CodeTable {
    // DFS traversal: left=0, right=1
}

func bitStreamToBytes(bits string) []byte {
    // Convert "010011..." to []byte
}
```

---

### **5.3 Quality Metrics (core/metrics/psnr.go, ssim.go)**

#### **5.3.1 PSNR (Peak Signal-to-Noise Ratio)**

```go
package metrics

import (
    "image"
    "math"
)

// CalculatePSNR computes quality metric between original and compressed
// Returns: PSNR in dB (higher = better, 30+ is acceptable, 40+ is excellent)
func CalculatePSNR(original, compressed image.Image) (float64, error) {
    if original.Bounds() != compressed.Bounds() {
        return 0, fmt.Errorf("image dimensions mismatch")
    }
    
    mse := calculateMSE(original, compressed)
    
    if mse == 0 {
        return math.Inf(1), nil // Identical images
    }
    
    maxPixelValue := 255.0
    psnr := 10 * math.Log10((maxPixelValue * maxPixelValue) / mse)
    
    return psnr, nil
}

// calculateMSE computes Mean Squared Error
func calculateMSE(original, compressed image.Image) float64 {
    bounds := original.Bounds()
    width := bounds.Dx()
    height := bounds.Dy()
    
    sumSquaredError := 0.0
    pixelCount := 0
    
    for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
        for x := bounds.Min.X; x < bounds.Max.X; x++ {
            // Get RGB values
            r1, g1, b1, _ := original.At(x, y).RGBA()
            r2, g2, b2, _ := compressed.At(x, y).RGBA()
            
            // Convert to 0-255 range
            r1, g1, b1 = r1>>8, g1>>8, b1>>8
            r2, g2, b2 = r2>>8, g2>>8, b2>>8
            
            // Calculate squared error for each channel
            sumSquaredError += math.Pow(float64(r1)-float64(r2), 2)
            sumSquaredError += math.Pow(float64(g1)-float64(g2), 2)
            sumSquaredError += math.Pow(float64(b1)-float64(b2), 2)
            
            pixelCount += 3 // RGB channels
        }
    }
    
    mse := sumSquaredError / float64(pixelCount)
    return mse
}
```

#### **5.3.2 SSIM (Structural Similarity Index)**

```go
// CalculateSSIM computes perceptual quality metric
// Returns: SSIM between 0-1 (1 = identical, >0.9 = excellent)
func CalculateSSIM(original, compressed image.Image) (float64, error) {
    // Window-based approach (11x11 Gaussian window)
    // Measures: luminance, contrast, structure
    
    // Constants
    k1 := 0.01
    k2 := 0.03
    L := 255.0 // Dynamic range
    
    c1 := (k1 * L) * (k1 * L)
    c2 := (k2 * L) * (k2 * L)
    
    // Calculate mean, variance, covariance in sliding windows
    // SSIM = ((2*μx*μy + c1)(2*σxy + c2)) / ((μx² + μy² + c1)(σx² + σy² + c2))
    
    // Implementation details...
}
```

---

### **5.4 Benchmark Service (services/benchmark_service.go)**

#### **5.4.1 Purpose**
Compare custom DCT implementation against standard libraries (Go, Python).

#### **5.4.2 Benchmark Targets**

```yaml
Go Libraries:
  - image/jpeg (stdlib)
  - github.com/chai2010/webp
  - Custom DCT implementation

Python Libraries:
  - Pillow (PIL)
  - OpenCV (cv2)
  - scikit-image
```

#### **5.4.3 Implementation**

```go
package services

import (
    "context"
    "fmt"
    "image"
    "time"
    
    "imagecompressor-dct/internal/core/dct"
    "imagecompressor-dct/internal/core/metrics"
    "imagecompressor-dct/internal/models"
    "imagecompressor-dct/internal/python"
)

// BenchmarkService orchestrates compression benchmarks
type BenchmarkService struct {
    dctEncoder    *dct.Encoder
    pythonBridge  *python.Bridge
}

// NewBenchmarkService creates benchmark service
func NewBenchmarkService(dctEncoder *dct.Encoder) *BenchmarkService {
    return &BenchmarkService{
        dctEncoder:   dctEncoder,
        pythonBridge: python.NewBridge(),
    }
}

// RunBenchmark compresses image with all methods
func (bs *BenchmarkService) RunBenchmark(
    ctx context.Context,
    originalImage image.Image,
    quality int,
) (*models.BenchmarkReport, error) {
    
    report := &models.BenchmarkReport{
        OriginalSize: calculateImageSize(originalImage),
        Quality:      quality,
        Results:      make([]models.BenchmarkResult, 0),
    }
    
    // 1. Custom DCT
    customResult, err := bs.benchmarkCustomDCT(originalImage, quality)
    if err != nil {
        return nil, fmt.Errorf("custom DCT benchmark failed: %w", err)
    }
    report.Results = append(report.Results, customResult)
    
    // 2. Go stdlib JPEG
    goJPEGResult, err := bs.benchmarkGoJPEG(originalImage, quality)
    if err != nil {
        return nil, fmt.Errorf("Go JPEG benchmark failed: %w", err)
    }
    report.Results = append(report.Results, goJPEGResult)
    
    // 3. WebP
    webpResult, err := bs.benchmarkWebP(originalImage, quality)
    if err != nil {
        return nil, fmt.Errorf("WebP benchmark failed: %w", err)
    }
    report.Results = append(report.Results, webpResult)
    
    // 4. Python PIL
    pilResult, err := bs.benchmarkPythonPIL(originalImage, quality)
    if err != nil {
        // Non-fatal: Python might not be available
        log.Printf("Python PIL benchmark skipped: %v", err)
    } else {
        report.Results = append(report.Results, pilResult)
    }
    
    // 5. Python OpenCV
    opencvResult, err := bs.benchmarkPythonOpenCV(originalImage, quality)
    if err != nil {
        log.Printf("Python OpenCV benchmark skipped: %v", err)
    } else {
        report.Results = append(report.Results, opencvResult)
    }
    
    return report, nil
}

// benchmarkCustomDCT runs custom implementation
func (bs *BenchmarkService) benchmarkCustomDCT(
    img image.Image,
    quality int,
) (models.BenchmarkResult, error) {
    
    startTime := time.Now()
    
    // Compress
    compressionResult, err := bs.dctEncoder.Encode(img)
    if err != nil {
        return models.BenchmarkResult{}, err
    }
    
    encodingTime := time.Since(startTime)
    
    // Decode (for PSNR calculation)
    decodedImage, err := bs.dctEncoder.Decode(compressionResult.Data)
    if err != nil {
        return models.BenchmarkResult{}, err
    }
    
    decodingTime := time.Since(startTime) - encodingTime
    
    // Calculate metrics
    psnr, _ := metrics.CalculatePSNR(img, decodedImage)
    ssim, _ := metrics.CalculateSSIM(img, decodedImage)
    
    return models.BenchmarkResult{
        Method:           "Custom DCT",
        Language:         "Go",
        CompressedSize:   compressionResult.CompressedSize,
        CompressionRatio: compressionResult.CompressionRatio,
        EncodingTime:     encodingTime,
        DecodingTime:     decodingTime,
        PSNR:             psnr,
        SSIM:             ssim,
    }, nil
}

// benchmarkGoJPEG runs Go stdlib JPEG encoder
func (bs *BenchmarkService) benchmarkGoJPEG(
    img image.Image,
    quality int,
) (models.BenchmarkResult, error) {
    // Use image/jpeg package
    // Similar structure to benchmarkCustomDCT
}

// benchmarkPythonPIL calls Python script
func (bs *BenchmarkService) benchmarkPythonPIL(
    img image.Image,
    quality int,
) (models.BenchmarkResult, error) {
    
    // Save image to temp file
    tempPath := "/tmp/benchmark_input.png"
    err := saveImage(img, tempPath)
    if err != nil {
        return models.BenchmarkResult{}, err
    }
    
    // Execute Python script
    result, err := bs.pythonBridge.ExecuteScript(
        "compress_pil.py",
        tempPath,
        quality,
    )
    
    if err != nil {
        return models.BenchmarkResult{}, err
    }
    
    return result, nil
}
```

---

### **5.5 Python Bridge (internal/python/bridge.go)**

```go
package python

import (
    "encoding/json"
    "fmt"
    "os/exec"
    "time"
    
    "imagecompressor-dct/internal/models"
)

// Bridge executes Python scripts and parses results
type Bridge struct {
    pythonPath  string
    scriptsPath string
}

// NewBridge creates Python executor
func NewBridge() *Bridge {
    return &Bridge{
        pythonPath:  "python3", // Or detect from PATH
        scriptsPath: "./internal/python/scripts",
    }
}

// ExecuteScript runs Python script and parses JSON output
func (b *Bridge) ExecuteScript(
    scriptName string,
    imagePath string,
    quality int,
) (models.BenchmarkResult, error) {
    
    scriptPath := fmt.Sprintf("%s/%s", b.scriptsPath, scriptName)
    
    // Build command
    cmd := exec.Command(
        b.pythonPath,
        scriptPath,
        "--input", imagePath,
        "--quality", fmt.Sprintf("%d", quality),
        "--output-json",
    )
    
    // Execute with timeout
    outputBytes, err := cmd.Output()
    if err != nil {
        return models.BenchmarkResult{}, fmt.Errorf("script failed: %w", err)
    }
    
    // Parse JSON
    var result models.BenchmarkResult
    err = json.Unmarshal(outputBytes, &result)
    if err != nil {
        return models.BenchmarkResult{}, fmt.Errorf("failed to parse output: %w", err)
    }
    
    return result, nil
}
```

#### **5.5.1 Python Script Example (internal/python/scripts/compress_pil.py)**

```python
#!/usr/bin/env python3
"""
Compress image using Pillow (PIL) and output metrics as JSON
"""

import argparse
import json
import time
from PIL import Image
from skimage.metrics import peak_signal_noise_ratio, structural_similarity
import numpy as np

def compress_with_pil(input_path, quality, output_path="/tmp/compressed_pil.jpg"):
    # Load image
    start_time = time.time()
    original = Image.open(input_path).convert('RGB')
    load_time = time.time() - start_time
    
    # Compress
    encode_start = time.time()
    original.save(output_path, "JPEG", quality=quality, optimize=True)
    encoding_time = time.time() - encode_start
    
    # Decode
    decode_start = time.time()
    compressed = Image.open(output_path)
    decoding_time = time.time() - decode_start
    
    # Calculate metrics
    original_np = np.array(original)
    compressed_np = np.array(compressed)
    
    psnr = peak_signal_noise_ratio(original_np, compressed_np, data_range=255)
    ssim = structural_similarity(original_np, compressed_np, multichannel=True, channel_axis=2)
    
    # Get file sizes
    import os
    original_size = os.path.getsize(input_path)
    compressed_size = os.path.getsize(output_path)
    
    # Build result
    result = {
        "method": "PIL JPEG",
        "language": "Python",
        "compressed_size": compressed_size,
        "compression_ratio": original_size / compressed_size,
        "encoding_time_ms": encoding_time * 1000,
        "decoding_time_ms": decoding_time * 1000,
        "psnr": psnr,
        "ssim": ssim
    }
    
    return result

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--quality", type=int, required=True)
    parser.add_argument("--output-json", action="store_true")
    
    args = parser.parse_args()
    
    result = compress_with_pil(args.input, args.quality)
    
    if args.output_json:
        print(json.dumps(result))
    else:
        print(f"Compressed: {result['compressed_size']} bytes")
        print(f"PSNR: {result['psnr']:.2f} dB")
```

---

## **6. IMPLEMENTATION ROADMAP**

### **6.1 Phase 1: Foundation (Week 1)**

#### **Day 1-2: Project Setup**
```bash
Tasks:
  - [ ] Create directory structure
  - [ ] Initialize Go module
  - [ ] Setup Makefile with common commands
  - [ ] Create README.md with project description
  - [ ] Configure .gitignore
  - [ ] Setup test infrastructure

Commands:
  $ mkdir -p imagecompressor-dct/{cmd,internal,web,test,docs,scripts}
  $ cd imagecompressor-dct
  $ go mod init imagecompressor-dct
  $ touch Makefile README.md .gitignore
```

**Makefile Example:**
```makefile
.PHONY: build test run clean

build:
	go build -o bin/server cmd/server/main.go

test:
	go test -v -cover ./...

run:
	go run cmd/server/main.go

benchmark:
	go test -bench=. -benchmem ./test/benchmarks/

clean:
	rm -rf bin/ tmp/

lint:
	golangci-lint run

format:
	gofmt -w -s .
```

#### **Day 3-4: Core DCT Implementation**
```yaml
Deliverables:
  - [ ] internal/core/dct/transform.go (2D DCT math)
  - [ ] internal/core/dct/quantization.go (tables)
  - [ ] internal/core/dct/encoder.go (main algorithm)
  - [ ] test/unit/dct_test.go (unit tests)

Success Criteria:
  - 8x8 DCT transform matches reference implementation
  - All tests pass with 100% coverage
  - Benchmark: <50ms for 8x8 block
```

#### **Day 5-7: Image I/O & Format Handlers**
```yaml
Deliverables:
  - [ ] internal/formats/png_handler.go
  - [ ] internal/formats/jpeg_handler.go
  - [ ] internal/formats/webp_handler.go
  - [ ] internal/io/image_loader.go
  - [ ] internal/io/image_writer.go

Success Criteria:
  - Load all supported formats without errors
  - Preserve color space information
  - Handle edge cases (grayscale, RGBA, etc.)
```

---

### **6.2 Phase 2: Compression Pipeline (Week 2)**

#### **Day 8-10: Complete Encoder**
```yaml
Tasks:
  - [ ] Implement chroma subsampling
  - [ ] Implement zigzag scan
  - [ ] Integrate Huffman encoding
  - [ ] Build .dct custom format writer

Success Criteria:
  - Compress 1920x1080 PNG in <1 second
  - Output file is valid and decodable
  - Compression ratio >5x at quality=50
```

#### **Day 11-12: Decoder Implementation**
```yaml
Deliverables:
  - [ ] internal/core/dct/decoder.go
  - [ ] Inverse DCT transform
  - [ ] Huffman decoding
  - [ ] Color space conversion (YCbCr → RGB)

Success Criteria:
  - Decode compressed images correctly
  - PSNR > 30dB at quality=50
  - Round-trip test passes
```

#### **Day 13-14: Quality Metrics**
```yaml
Deliverables:
  - [ ] internal/core/metrics/psnr.go
  - [ ] internal/core/metrics/ssim.go
  - [ ] internal/core/metrics/mse.go

Success Criteria:
  - PSNR calculation matches reference
  - SSIM calculation matches scikit-image
  - Performance: <100ms for 1080p image
```

---

### **6.3 Phase 3: Benchmarking (Week 3)**

#### **Day 15-17: Python Integration**
```yaml
Deliverables:
  - [ ] internal/python/bridge.go
  - [ ] internal/python/scripts/compress_pil.py
  - [ ] internal/python/scripts/compress_opencv.py
  - [ ] internal/python/scripts/calculate_metrics.py

Success Criteria:
  - Python scripts execute successfully
  - JSON parsing works correctly
  - Error handling for missing Python
```

#### **Day 18-19: Benchmark Service**
```yaml
Deliverables:
  - [ ] services/benchmark_service.go
  - [ ] Integrate all compression methods
  - [ ] Generate comparison reports

Success Criteria:
  - Compare 5+ compression methods
  - Output CSV/JSON reports
  - Visualize results in web UI
```

#### **Day 20-21: Go Benchmark Suite**
```yaml
Deliverables:
  - [ ] test/benchmarks/compression_bench_test.go
  - [ ] Benchmark DCT vs stdlib JPEG
  - [ ] Memory profiling

Example:
  func BenchmarkDCTEncode(b *testing.B) {
      img := loadTestImage()
      encoder := dct.NewEncoder(...)
      
      b.ResetTimer()
      for i := 0; i < b.N; i++ {
          encoder.Encode(img)
      }
  }
```

---

### **6.4 Phase 4: Web Interface (Week 4)**

#### **Day 22-24: Backend API**
```yaml
Deliverables:
  - [ ] cmd/server/main.go (HTTP server)
  - [ ] internal/handlers/upload_handler.go
  - [ ] internal/handlers/compress_handler.go
  - [ ] internal/handlers/benchmark_handler.go

API Endpoints:
  POST /api/upload         - Upload image
  POST /api/compress       - Compress with options
  POST /api/benchmark      - Run full benchmark
  GET  /api/formats        - List supported formats
```

#### **Day 25-27: Frontend UI**
```yaml
Deliverables:
  - [ ] web/index.html (single-page app)
  - [ ] web/styles.css (dark mode support)
  - [ ] web/app.js (vanilla JS)

Features:
  - Drag-and-drop upload
  - Quality slider (1-100)
  - Chroma subsampling selector
  - Side-by-side comparison
  - Metrics display (PSNR, SSIM, size, time)
  - Download compressed image
  - Benchmark results table
```

#### **Day 28: Polish & Testing**
```yaml
Tasks:
  - [ ] Cross-browser testing
  - [ ] Mobile responsive design
  - [ ] Loading indicators
  - [ ] Error messages
  - [ ] Accessibility (ARIA labels)
```

---

### **6.5 Phase 5: Production Ready (Week 5)**

#### **Day 29-30: Documentation**
```yaml
Deliverables:
  - [ ] README.md (setup, usage, examples)
  - [ ] docs/ARCHITECTURE.md (this document)
  - [ ] docs/API.md (endpoint documentation)
  - [ ] docs/ALGORITHM.md (DCT explanation)
  - [ ] Inline code comments (godoc)
```

#### **Day 31-32: Performance Optimization**
```yaml
Tasks:
  - [ ] Profile with pprof
  - [ ] Optimize hot paths
  - [ ] Reduce memory allocations
  - [ ] Parallel processing (goroutines)
  - [ ] Caching frequently used data
```

#### **Day 33-34: Final Testing**
```yaml
Test Coverage:
  - [ ] Unit tests: 100%
  - [ ] Integration tests: All workflows
  - [ ] Benchmark tests: Performance baseline
  - [ ] Load testing: 100 concurrent requests
  - [ ] Edge cases: Corrupted images, invalid input
```

#### **Day 35: Deployment**
```yaml
Deliverables:
  - [ ] Docker containerization (optional)
  - [ ] Deployment script
  - [ ] Usage examples
  - [ ] Performance report
```

---

## **7. API SPECIFICATIONS**

### **7.1 REST API Endpoints**

#### **7.1.1 Upload Image**
```http
POST /api/upload
Content-Type: multipart/form-data

Request Body:
{
  "image": <file> (PNG/JPEG/WebP/BMP/TIFF, max 10MB)
}

Response (200 OK):
{
  "image_id": "abc123",
  "format": "PNG",
  "width": 1920,
  "height": 1080,
  "size_bytes": 5242880,
  "color_mode": "RGB"
}

Error Response (400 Bad Request):
{
  "error": "Unsupported format",
  "message": "Only PNG, JPEG, WebP, BMP, TIFF are supported"
}
```

#### **7.1.2 Compress Image**
```http
POST /api/compress
Content-Type: application/json

Request Body:
{
  "image_id": "abc123",
  "quality": 75,                    // 1-100
  "chroma_subsampling": "4:2:0",   // "4:4:4", "4:2:2", "4:2:0"
  "output_format": "dct"            // "dct", "jpeg", "webp"
}

Response (200 OK):
{
  "compressed_image_url": "/api/download/xyz789",
  "original_size": 5242880,
  "compressed_size": 524288,
  "compression_ratio": 10.0,
  "encoding_time_ms": 245,
  "psnr": 35.6,
  "ssim": 0.94
}
```

#### **7.1.3 Run Benchmark**
```http
POST /api/benchmark
Content-Type: application/json

Request Body:
{
  "image_id": "abc123",
  "quality": 75,
  "methods": ["custom_dct", "go_jpeg", "webp", "python_pil"]
}

Response (200 OK):
{
  "benchmark_id": "bench_456",
  "results": [
    {
      "method": "Custom DCT",
      "language": "Go",
      "compressed_size": 524288,
      "compression_ratio": 10.0,
      "encoding_time_ms": 245,
      "decoding_time_ms": 180,
      "psnr": 35.6,
      "ssim": 0.94
    },
    {
      "method": "Go stdlib JPEG",
      "language": "Go",
      "compressed_size": 655360,
      "compression_ratio": 8.0,
      "encoding_time_ms": 150,
      "decoding_time_ms": 120,
      "psnr": 33.2,
      "ssim": 0.91
    },
    // ... more results
  ],
  "csv_download_url": "/api/benchmark/bench_456/download"
}
```

#### **7.1.4 Download Compressed Image**
```http
GET /api/download/:file_id
Response: Binary image data
Content-Type: image/jpeg (or appropriate)
```

---

## **8. UI/UX REQUIREMENTS**

### **8.1 Single-Page Web Interface**

#### **8.1.1 Layout Structure**
```
┌─────────────────────────────────────────────────────────────┐
│  Header: ImageCompressor-DCT                    [Dark Mode] │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────────────────┐   ┌─────────────────────────┐ │
│  │  Upload Area            │   │  Settings Panel          │ │
│  │  [Drag & Drop]          │   │  Quality: [====|===] 75  │ │
│  │  or                      │   │  Subsampling: [4:2:0▼]  │ │
│  │  [Browse Files]          │   │  Output: [DCT▼]         │ │
│  │                          │   │  [Compress Button]       │ │
│  └─────────────────────────┘   └─────────────────────────┘ │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  Comparison View                                       │ │
│  │  ┌──────────────┐         ┌──────────────┐           │ │
│  │  │  Original    │         │  Compressed  │           │ │
│  │  │  5.0 MB      │         │  500 KB      │           │ │
│  │  │              │         │              │           │ │
│  │  └──────────────┘         └──────────────┘           │ │
│  │  PSNR: 35.6 dB    SSIM: 0.94    Time: 245ms          │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐ │
│  │  Benchmark Results                                     │ │
│  │  Method        | Size   | Ratio | PSNR  | Time        │ │
│  │  Custom DCT    | 500 KB | 10.0x | 35.6  | 245ms       │ │
│  │  Go JPEG       | 655 KB | 8.0x  | 33.2  | 150ms       │ │
│  │  WebP          | 480 KB | 10.9x | 36.1  | 320ms       │ │
│  │  [Download CSV] [View Details]                        │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

#### **8.1.2 Color Scheme (Dark Mode)**
```css
:root {
  /* Dark theme */
  --bg-primary: #1a1a1a;
  --bg-secondary: #2d2d2d;
  --text-primary: #e0e0e0;
  --text-secondary: #a0a0a0;
  --accent-primary: #4a9eff;
  --accent-hover: #6bb1ff;
  --success: #4caf50;
  --warning: #ff9800;
  --error: #f44336;
  
  /* Light theme (optional toggle) */
  --bg-primary-light: #ffffff;
  --bg-secondary-light: #f5f5f5;
  --text-primary-light: #212121;
}
```

#### **8.1.3 Interactive Elements**

**Quality Slider:**
```html
<div class="quality-control">
  <label for="quality">Quality: <span id="quality-value">75</span></label>
  <input 
    type="range" 
    id="quality" 
    min="1" 
    max="100" 
    value="75"
    oninput="updateQualityPreview(this.value)"
  >
  <div class="quality-labels">
    <span>Low</span>
    <span>Medium</span>
    <span>High</span>
  </div>
</div>
```

**Drag-and-Drop:**
```javascript
const dropZone = document.getElementById('upload-area');

dropZone.addEventListener('dragover', (event) => {
  event.preventDefault();
  dropZone.classList.add('drag-active');
});

dropZone.addEventListener('drop', (event) => {
  event.preventDefault();
  const files = event.dataTransfer.files;
  handleImageUpload(files[0]);
});
```

---

## **9. BENCHMARKING REQUIREMENTS**

### **9.1 Metrics to Measure**

```yaml
Quality Metrics:
  - PSNR (Peak Signal-to-Noise Ratio): >30 dB acceptable, >40 dB excellent
  - SSIM (Structural Similarity): >0.9 acceptable, >0.95 excellent
  - MSE (Mean Squared Error): Lower is better

Size Metrics:
  - Original file size (bytes)
  - Compressed file size (bytes)
  - Compression ratio (original/compressed)
  - Bits per pixel

Performance Metrics:
  - Encoding time (milliseconds)
  - Decoding time (milliseconds)
  - Memory usage (MB)
  - CPU usage (%)

Comparison Metrics:
  - Size delta vs Go JPEG (%)
  - PSNR delta vs Go JPEG (dB)
  - Speed delta vs Go JPEG (%)
```

### **9.2 Test Image Dataset**

```yaml
Required Test Images:
  - Lena (512x512, standard test image)
  - Kodak PhotoCD (768x512, 24 images)
  - High-resolution landscape (4K, 3840x2160)
  - Portrait photo (1080x1920)
  - Screenshot (text-heavy, 1920x1080)
  - Gradient image (smooth color transitions)
  - Noise image (high entropy)

Sources:
  - USC-SIPI Image Database
  - Kodak Lossless True Color Image Suite
  - Create synthetic images programmatically
```

### **9.3 Benchmark Report Format**

#### **9.3.1 CSV Output**
```csv
Method,Language,OriginalSize,CompressedSize,Ratio,PSNR,SSIM,EncodeTime,DecodeTime
Custom DCT,Go,5242880,524288,10.0,35.6,0.94,245,180
Go JPEG,Go,5242880,655360,8.0,33.2,0.91,150,120
WebP,Go,5242880,480000,10.9,36.1,0.93,320,250
Python PIL,Python,5242880,650000,8.1,33.5,0.92,280,200
```

#### **9.3.2 JSON Output**
```json
{
  "benchmark_id": "bench_123",
  "timestamp": "2024-04-24T10:30:00Z",
  "test_image": {
    "name": "landscape_4k.png",
    "width": 3840,
    "height": 2160,
    "original_size": 5242880,
    "format": "PNG"
  },
  "settings": {
    "quality": 75,
    "chroma_subsampling": "4:2:0"
  },
  "results": [
    {
      "method": "Custom DCT",
      "language": "Go",
      "compressed_size": 524288,
      "compression_ratio": 10.0,
      "encoding_time_ms": 245,
      "decoding_time_ms": 180,
      "psnr": 35.6,
      "ssim": 0.94,
      "memory_mb": 45
    }
  ]
}
```

---

## **10. CODE QUALITY STANDARDS**

### **10.1 Naming Conventions**

```go
// GOOD Examples

// Package names: lowercase, singular
package dct
package metrics

// File names: snake_case
// compression_service.go
// image_loader.go

// Exported functions: PascalCase
func CalculatePSNR(original, compressed image.Image) (float64, error)
func NewEncoder(options CompressionOptions) *Encoder

// Internal functions: camelCase
func calculateMSE(img1, img2 image.Image) float64
func buildHuffmanTree(frequencies map[int]int) *Node

// Constants: PascalCase (exported) or camelCase (internal)
const MaxImageSize = 10 * 1024 * 1024 // 10MB
const defaultQuality = 75

// Variables: descriptive, human-readable
originalImageData := loadImage(path)
compressionStartTime := time.Now()
quantizedDCTCoefficients := quantize(dctBlocks)

// NOT single letters except loops
for blockIndex := 0; blockIndex < totalBlocks; blockIndex++ { // ✅
for i := 0; i < len(blocks); i++ { // ✅ (loop counter)
for x := 0; x < 8; x++ { // ✅ (coordinate)

data := d // ❌ NO
img := i // ❌ NO (unless truly obvious)
```

### **10.2 Documentation Requirements**

```go
// Package-level comment (MANDATORY for every package)
// Package dct implements Discrete Cosine Transform-based image compression
// following the JPEG baseline specification. It provides both encoding
// and decoding functionality with tunable quality parameters.
package dct

// Exported function documentation (MANDATORY)
// CalculatePSNR computes the Peak Signal-to-Noise Ratio between two images.
// PSNR is a quality metric measured in decibels (dB), where higher values
// indicate better quality. Values above 30 dB are generally acceptable for
// lossy compression, while values above 40 dB are considered excellent.
//
// Parameters:
//   - original: The reference image (uncompressed)
//   - compressed: The processed image to evaluate
//
// Returns:
//   - PSNR in decibels
//   - error if images have different dimensions
//
// Example:
//
//	psnr, err := CalculatePSNR(originalImg, compressedImg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Quality: %.2f dB\n", psnr)
func CalculatePSNR(original, compressed image.Image) (float64, error) {
    // Implementation
}

// Struct documentation
// CompressionOptions defines tunable parameters for DCT encoding.
// Quality ranges from 1 (maximum compression, lowest quality) to 100
// (minimum compression, highest quality). ChromaSubsampling reduces color
// information resolution, with "4:2:0" providing the most compression.
type CompressionOptions struct {
    Quality           int    // 1-100, higher = better quality
    ChromaSubsampling string // "4:4:4", "4:2:2", or "4:2:0"
    BlockSize         int    // DCT block dimension (usually 8)
}

// Internal function comments (OPTIONAL but encouraged)
// quantizeSingleBlock divides DCT coefficients by quantization table values.
// This is where lossy compression occurs - higher quality settings use
// smaller divisors, preserving more information.
func (e *Encoder) quantizeSingleBlock(block [][]float64) [][]int {
    // Implementation
}
```

### **10.3 Error Handling**

```go
// MANDATORY: Always check errors, never ignore

// ✅ GOOD
imageData, err := loadImage(path)
if err != nil {
    return nil, fmt.Errorf("failed to load image from %s: %w", path, err)
}

// ✅ GOOD: Wrap errors with context
func (s *CompressionService) CompressImage(path string) error {
    img, err := s.loader.Load(path)
    if err != nil {
        return fmt.Errorf("compression failed during image loading: %w", err)
    }
    
    result, err := s.encoder.Encode(img)
    if err != nil {
        return fmt.Errorf("compression failed during encoding: %w", err)
    }
    
    return nil
}

// ❌ BAD: Ignoring errors
imageData, _ := loadImage(path) // NEVER DO THIS

// ❌ BAD: Generic error messages
if err != nil {
    return errors.New("error") // NOT HELPFUL
}

// ❌ BAD: Using panic in library code
if err != nil {
    panic(err) // ONLY in main() or tests
}
```

### **10.4 Testing Requirements**

```go
// Unit test structure
func TestCalculatePSNR(t *testing.T) {
    // Arrange: Setup test data
    original := createTestImage(100, 100, color.RGBA{255, 0, 0, 255})
    compressed := createTestImage(100, 100, color.RGBA{250, 0, 0, 255})
    
    // Act: Execute function
    psnr, err := CalculatePSNR(original, compressed)
    
    // Assert: Verify results
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    expectedPSNR := 40.0 // Approximate
    tolerance := 1.0
    if math.Abs(psnr-expectedPSNR) > tolerance {
        t.Errorf("PSNR mismatch: got %.2f, want %.2f ±%.2f", 
            psnr, expectedPSNR, tolerance)
    }
}

// Table-driven tests (PREFERRED for multiple cases)
func TestDCTTransform(t *testing.T) {
    tests := []struct {
        name     string
        input    [][]float64
        expected [][]float64
    }{
        {
            name:     "zero block",
            input:    createZeroBlock(8, 8),
            expected: createZeroBlock(8, 8),
        },
        {
            name:     "constant block",
            input:    createConstantBlock(8, 8, 128),
            expected: createDCOnlyBlock(8, 8, 1024),
        },
        // More test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := applyDCT2D(tt.input)
            assertMatrixEqual(t, result, tt.expected, 0.01)
        })
    }
}

// Benchmark tests (MANDATORY for performance-critical code)
func BenchmarkDCT8x8(b *testing.B) {
    block := createRandomBlock(8, 8)
    
    b.ResetTimer() // Don't include setup time
    for i := 0; i < b.N; i++ {
        applyDCT2D(block)
    }
}

// Example test output
// --- PASS: TestCalculatePSNR (0.05s)
// BenchmarkDCT8x8-8   50000   25000 ns/op   1024 B/op   8 allocs/op
```

---

## **11. TESTING STRATEGY**

### **11.1 Test Coverage Targets**

```yaml
Unit Tests:
  - Core algorithms: 100% coverage MANDATORY
  - Services: 90%+ coverage
  - Handlers: 80%+ coverage
  - Utilities: 100% coverage

Integration Tests:
  - End-to-end compression workflow
  - API endpoint testing
  - Multi-format support

Benchmark Tests:
  - All performance-critical functions
  - Memory allocation tracking
  - Comparison against baselines
```

### **11.2 Test Organization**

```
test/
├── unit/
│   ├── dct_test.go              # Core algorithm tests
│   ├── huffman_test.go
│   ├── metrics_test.go
│   └── quantization_test.go
│
├── integration/
│   ├── compression_flow_test.go # Full workflow tests
│   ├── api_test.go              # HTTP endpoint tests
│   └── benchmark_test.go        # Benchmark orchestration
│
├── benchmarks/
│   ├── compression_bench_test.go
│   └── dct_bench_test.go
│
└── testdata/
    ├── images/
    │   ├── lena.png
    │   ├── landscape_4k.png
    │   └── text_screenshot.png
    └── expected/
        └── lena_compressed_q75.dct
```

### **11.3 Running Tests**

```bash
# All tests
make test

# Specific package
go test -v ./internal/core/dct/

# With coverage
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Benchmarks
go test -bench=. -benchmem ./test/benchmarks/

# Race detection
go test -race ./...

# Verbose output
go test -v -run TestDCTTransform
```

---

## **12. PERFORMANCE TARGETS**

### **12.1 Latency Requirements**

```yaml
Image Processing:
  - 1920x1080 PNG encode: <500ms
  - 1920x1080 PNG decode: <300ms
  - 3840x2160 (4K) encode: <2000ms
  
API Response Times:
  - Upload endpoint: <200ms
  - Compress endpoint: <1000ms (including processing)
  - Benchmark endpoint: <5000ms (all methods)

DCT Operations:
  - 8x8 block DCT: <50μs
  - Quantization: <10μs per block
  - Huffman encoding: <100ms for full image
```

### **12.2 Memory Constraints**

```yaml
Peak Memory Usage:
  - 1920x1080 image: <100MB
  - 3840x2160 (4K) image: <400MB
  - Server baseline: <50MB

Allocations:
  - Minimize heap allocations in hot paths
  - Pre-allocate buffers where possible
  - Reuse slices and arrays
```

### **12.3 Optimization Checklist**

```yaml
Pre-Optimization:
  - [ ] Profile with pprof (CPU and memory)
  - [ ] Identify hot paths (functions taking >10% CPU)
  - [ ] Measure baseline performance

Optimization Techniques:
  - [ ] Pre-compute lookup tables (cosine values)
  - [ ] Use sync.Pool for temporary buffers
  - [ ] Parallel processing with goroutines
  - [ ] SIMD instructions (optional, advanced)
  - [ ] Memory-mapped I/O for large files

Post-Optimization:
  - [ ] Re-profile and verify improvements
  - [ ] Ensure correctness (tests still pass)
  - [ ] Document optimization decisions
```

---

## **13. DEPLOYMENT GUIDE**

### **13.1 Build Instructions**

```bash
# Development build
make build

# Production build (optimizations enabled)
go build -ldflags="-s -w" -o bin/server cmd/server/main.go

# Cross-compilation (Linux, macOS, Windows)
GOOS=linux GOARCH=amd64 go build -o bin/server-linux cmd/server/main.go
GOOS=darwin GOARCH=amd64 go build -o bin/server-macos cmd/server/main.go
GOOS=windows GOARCH=amd64 go build -o bin/server.exe cmd/server/main.go
```

### **13.2 Running the Server**

```bash
# Default (localhost:8080)
./bin/server

# Custom port
./bin/server --port 3000

# With configuration file
./bin/server --config config.yaml
```

### **13.3 Configuration**

```yaml
# config.yaml
server:
  port: 8080
  host: 0.0.0.0
  
compression:
  max_upload_size: 10485760  # 10MB
  default_quality: 75
  allowed_formats:
    - png
    - jpeg
    - webp
    - bmp
    - tiff

benchmark:
  enabled: true
  python_path: /usr/bin/python3
  timeout_seconds: 30

storage:
  temp_dir: /tmp/imagecompressor
  cleanup_interval: 3600  # 1 hour
```

### **13.4 Docker Deployment (Optional)**

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o server cmd/server/main.go

# Runtime image
FROM python:3.11-slim

RUN pip install Pillow opencv-python-headless scikit-image numpy

WORKDIR /app
COPY --from=builder /app/server .
COPY web/ ./web/
COPY internal/python/scripts/ ./internal/python/scripts/

EXPOSE 8080
CMD ["./server"]
```

```bash
# Build and run
docker build -t imagecompressor-dct .
docker run -p 8080:8080 imagecompressor-dct
```

---

## **14. LEARNING OUTCOMES**

### **14.1 Computer Science Concepts Mastered**

```yaml
Data Structures:
  - Binary trees (Huffman tree)
  - Hash maps (frequency tables, code tables)
  - 2D arrays (image representation, DCT blocks)
  - Priority queues (Huffman tree construction)

Algorithms:
  - Discrete Cosine Transform (signal processing)
  - Huffman encoding (greedy algorithm)
  - Quantization (lossy compression)
  - Zigzag scanning (spatial locality)
  - Binary search (time index queries)

Mathematical Concepts:
  - Linear algebra (matrix transformations)
  - Trigonometry (cosine functions in DCT)
  - Statistics (MSE, PSNR, SSIM)
  - Information theory (entropy, compression)

Performance Optimization:
  - Algorithmic complexity (O(n log n) Huffman)
  - Memory management (pre-allocation, pooling)
  - Profiling and benchmarking
  - Parallel processing (goroutines)
```

### **14.2 Go Language Skills**

```yaml
Core Language:
  - Structs and methods
  - Interfaces and polymorphism
  - Error handling patterns
  - Goroutines and channels
  - Context for cancellation

Standard Library:
  - image package (PNG, JPEG encoding/decoding)
  - net/http (web server, handlers)
  - encoding/json (API responses)
  - math (floating-point operations)
  - testing (unit tests, benchmarks)

Best Practices:
  - Clean architecture (separation of concerns)
  - Dependency injection
  - Table-driven tests
  - Effective error wrapping
  - Documentation with godoc
```

### **14.3 Software Engineering Skills**

```yaml
Architecture & Design:
  - Layered architecture (presentation, application, domain, infrastructure)
  - Repository pattern (data access abstraction)
  - Service layer (business logic orchestration)
  - API design (RESTful endpoints)

Testing & Quality:
  - Unit testing (isolated components)
  - Integration testing (end-to-end workflows)
  - Benchmark testing (performance measurement)
  - Test coverage analysis

Tools & DevOps:
  - Version control (Git)
  - Build automation (Makefiles)
  - Containerization (Docker)
  - Cross-compilation
```

### **14.4 Domain Knowledge**

```yaml
Image Processing:
  - Color spaces (RGB, YCbCr)
  - Chroma subsampling (4:4:4, 4:2:2, 4:2:0)
  - Lossy vs lossless compression
  - Quality metrics (PSNR, SSIM)

Compression Standards:
  - JPEG baseline algorithm
  - DCT-based compression
  - Huffman coding
  - Quantization tables

Benchmarking:
  - Cross-language comparison (Go vs Python)
  - Statistical significance
  - Performance profiling
  - Report generation
```

### **14.5 Resume Talking Points**

**For Interviews:**

1. **Algorithmic Depth**: 
   - "Implemented 2D Discrete Cosine Transform from mathematical specification, achieving <50μs processing time per 8x8 block."

2. **Performance Optimization**:
   - "Optimized DCT encoder to compress 1080p images in <500ms by pre-computing lookup tables and using memory pooling."

3. **Cross-Language Integration**:
   - "Built Python bridge to benchmark Go implementation against PIL/OpenCV, handling subprocess execution and JSON parsing."

4. **Quality Metrics**:
   - "Implemented PSNR and SSIM calculation algorithms, validating against scikit-image reference implementation."

5. **Clean Architecture**:
   - "Designed layered architecture separating core algorithms from I/O and API layers, enabling 100% unit test coverage of compression logic."

6. **Full-Stack Development**:
   - "Built interactive web UI with drag-drop upload, real-time quality preview, and side-by-side comparison visualizations."

### **14.6 Key Project Highlights for Resume**

```markdown
## Image Compression Engine (Go, Python, WebAssembly)

Developed a production-grade multi-format image compression system implementing JPEG-like DCT algorithm with performance benchmarking against industry standards.

**Technical Implementation:**
- Implemented Discrete Cosine Transform (DCT) encoder/decoder from mathematical specification
- Built Huffman entropy coder with frequency analysis and tree construction
- Integrated chroma subsampling (4:4:4, 4:2:2, 4:2:0) and adaptive quantization
- Achieved 10x compression ratio with 35+ dB PSNR quality at medium settings

**Performance & Optimization:**
- Optimized DCT calculation to <50μs per 8x8 block using lookup tables
- Compressed 1920x1080 images in <500ms with <100MB memory footprint
- Implemented parallel processing with goroutines for batch operations
- Profiled with pprof and achieved 3x speedup through memory pooling

**Benchmarking & Validation:**
- Built cross-language benchmark comparing Go, Python (PIL/OpenCV), and WebP
- Integrated PSNR (Peak Signal-to-Noise Ratio) and SSIM quality metrics
- Generated comparative analysis reports (CSV/JSON) with 6+ performance metrics
- Validated algorithm correctness against JPEG reference implementation

**Architecture & Code Quality:**
- Designed clean layered architecture (repository pattern, service layer)
- Achieved 100% unit test coverage on core compression algorithms
- Implemented RESTful API with upload, compression, and benchmark endpoints
- Built responsive single-page web UI with real-time quality preview

**Technologies:** Go 1.21, Python 3.11, HTML5/CSS3/Vanilla JS, Docker
**Key Libraries:** image (stdlib), Pillow, OpenCV, scikit-image

**Results:** Achieved 20-30% better compression ratio vs Go stdlib JPEG while maintaining competitive encoding speed. Demonstrated deep understanding of signal processing, algorithm optimization, and cross-platform integration.
```

---

## **15. FINAL IMPLEMENTATION CHECKLIST**

### **15.1 Core Features (MANDATORY)**

```yaml
Phase 1 - Foundation:
  - [ ] Project structure created
  - [ ] Go module initialized
  - [ ] Makefile configured
  - [ ] README.md written

Phase 2 - DCT Algorithm:
  - [ ] 2D DCT transform implemented
  - [ ] Quantization tables created
  - [ ] Chroma subsampling added
  - [ ] Encoder complete
  - [ ] Decoder complete
  - [ ] Unit tests pass (100% coverage)

Phase 3 - Compression Pipeline:
  - [ ] Multi-format input (PNG, JPEG, WebP, BMP, TIFF)
  - [ ] Custom .dct format
  - [ ] Export to JPEG/WebP
  - [ ] Quality slider (1-100)
  - [ ] Huffman encoding

Phase 4 - Quality Metrics:
  - [ ] PSNR calculation
  - [ ] SSIM calculation
  - [ ] MSE calculation

Phase 5 - Benchmarking:
  - [ ] Go stdlib JPEG comparison
  - [ ] WebP comparison
  - [ ] Python PIL integration
  - [ ] Python OpenCV integration
  - [ ] CSV/JSON report generation

Phase 6 - Web Interface:
  - [ ] HTTP server (cmd/server/main.go)
  - [ ] Upload endpoint
  - [ ] Compress endpoint
  - [ ] Benchmark endpoint
  - [ ] Single-page UI (web/index.html)
  - [ ] Drag-and-drop upload
  - [ ] Side-by-side comparison
  - [ ] Dark mode support

Phase 7 - Testing:
  - [ ] Unit tests (100% for core)
  - [ ] Integration tests
  - [ ] Benchmark tests
  - [ ] Cross-browser testing

Phase 8 - Documentation:
  - [ ] Inline code comments (godoc)
  - [ ] README.md
  - [ ] API documentation
  - [ ] Architecture documentation
  - [ ] Algorithm explanation

Phase 9 - Performance:
  - [ ] Profiling complete
  - [ ] Optimization applied
  - [ ] Performance targets met
  - [ ] Memory constraints satisfied

Phase 10 - Deployment:
  - [ ] Build scripts
  - [ ] Docker support (optional)
  - [ ] Configuration file
  - [ ] Deployment guide
```

### **15.2 Code Quality Gates (MUST PASS)**

```yaml
Linting:
  - [ ] golangci-lint passes with zero warnings
  - [ ] gofmt -s applied to all files
  - [ ] No magic numbers (constants defined)
  - [ ] No global variables (except constants)

Testing:
  - [ ] All tests pass: go test ./...
  - [ ] Coverage >90%: go test -cover ./...
  - [ ] Race detector clean: go test -race ./...
  - [ ] Benchmarks run: go test -bench=. ./test/benchmarks/

Documentation:
  - [ ] Every exported function has godoc comment
  - [ ] Every package has package comment
  - [ ] README.md is complete
  - [ ] API endpoints documented

Performance:
  - [ ] 1080p compress <500ms
  - [ ] PSNR calculation <100ms
  - [ ] Memory <100MB for 1080p
  - [ ] No memory leaks (verified with pprof)
```

---

## **16. STRICT IMPLEMENTATION RULES**

### **16.1 Code Review Checklist (Before Committing)**

```yaml
Every Pull Request MUST:
  - [ ] Pass all unit tests
  - [ ] Pass all integration tests
  - [ ] Have zero linter warnings
  - [ ] Include test coverage for new code
  - [ ] Have descriptive commit messages
  - [ ] Update documentation if API changed

Every Function MUST:
  - [ ] Have descriptive name (no abbreviations)
  - [ ] Have godoc comment (if exported)
  - [ ] Return error (not panic) on failure
  - [ ] Validate input parameters
  - [ ] Use human-readable variable names

Every Test MUST:
  - [ ] Follow Arrange-Act-Assert pattern
  - [ ] Have descriptive test name
  - [ ] Test edge cases (nil, empty, overflow)
  - [ ] Clean up resources (defer cleanup)
```

### **16.2 Performance Requirements (NON-NEGOTIABLE)**

```yaml
Latency Targets:
  - 8x8 DCT transform: <50μs
  - 1920x1080 encode: <500ms
  - 1920x1080 decode: <300ms
  - API response: <1000ms

Memory Limits:
  - 1080p image processing: <100MB peak
  - 4K image processing: <400MB peak
  - Server baseline: <50MB
  - No memory leaks (verified with pprof)

Quality Targets:
  - Quality=50: PSNR >30 dB
  - Quality=75: PSNR >35 dB
  - Quality=90: PSNR >40 dB
  - Compression ratio >5x at quality=50
```

### **16.3 Security Requirements**

```yaml
Input Validation:
  - [ ] Max file size: 10MB (configurable)
  - [ ] Allowed formats: PNG, JPEG, WebP, BMP, TIFF only
  - [ ] Image dimension limits: max 8192x8192
  - [ ] Validate MIME type, not just extension

API Security:
  - [ ] Rate limiting (100 req/min per IP)
  - [ ] CORS configured properly
  - [ ] No path traversal vulnerabilities
  - [ ] Sanitize all user inputs

File Handling:
  - [ ] Use temporary directory with cleanup
  - [ ] Delete uploaded files after processing
  - [ ] No arbitrary file writes
  - [ ] Validate file paths
```

---

## **17. SMART IMPLEMENTATION TIPS**

### **17.1 Development Workflow**

```yaml
Day-to-Day Process:
  1. Write failing test first (TDD)
  2. Implement minimum code to pass test
  3. Refactor for clarity
  4. Run full test suite
  5. Commit with descriptive message

Git Workflow:
  - Feature branch: feature/dct-encoder
  - Commit message: "Add 2D DCT transform with unit tests"
  - Small, atomic commits
  - Never commit broken code

Debugging:
  - Use Go debugger (Delve): dlv debug
  - Add logging: log.Printf("Processing block %d", i)
  - Profile performance: go tool pprof
  - Visualize data: save intermediate images
```

### **17.2 Common Pitfalls to Avoid**

```yaml
❌ AVOID:
  - Premature optimization (measure first!)
  - Global variables (use dependency injection)
  - Ignoring errors (_ = function())
  - Magic numbers (16, 8, 255 without constants)
  - Long functions (>50 lines, split them)
  - Deep nesting (>3 levels, refactor)
  - Single-letter variables (except i, j in loops)

✅ DO:
  - Profile before optimizing
  - Write tests first
  - Use meaningful names
  - Document complex logic
  - Handle all errors
  - Keep functions small and focused
```

### **17.3 Time-Saving Shortcuts**

```yaml
Development Tools:
  - Use gopls (Go language server) for IDE integration
  - Install golangci-lint for automated checks
  - Use goimports to auto-format and organize imports
  - Set up file watchers for auto-reload

Code Generation:
  - Generate test boilerplate: gotests -all -w file.go
  - Generate mocks: mockgen for interfaces
  - Generate benchmarks: write once, copy pattern

Testing:
  - Use testdata/ directory for test images
  - Pre-compute expected results (save time)
  - Use table-driven tests (write once, test many)
```

---

## **18. FINAL REMINDERS**

### **18.1 Project Success Criteria**

```yaml
Technical Excellence:
  ✅ DCT algorithm matches JPEG specification
  ✅ Compression ratio >5x at quality=50
  ✅ PSNR >30 dB at quality=50
  ✅ Processing time <500ms for 1080p
  ✅ 100% test coverage on core algorithms

Code Quality:
  ✅ Zero linter warnings
  ✅ All tests pass
  ✅ Comprehensive documentation
  ✅ Clean architecture
  ✅ Human-readable variable names

Benchmarking:
  ✅ Compare against 4+ libraries
  ✅ Generate detailed reports
  ✅ Cross-language validation
  ✅ Performance profiling complete

User Experience:
  ✅ Intuitive single-page UI
  ✅ Drag-and-drop upload
  ✅ Real-time preview
  ✅ Side-by-side comparison
  ✅ Dark mode support
```

### **18.2 Resume-Worthy Achievements**

```markdown
When complete, you will have:
- ✅ Implemented a complete compression algorithm from math specification
- ✅ Built a multi-format image processing pipeline
- ✅ Integrated cross-language benchmarking (Go + Python)
- ✅ Achieved measurable performance improvements (20-30% better ratio)
- ✅ Demonstrated clean architecture and 100% test coverage
- ✅ Created production-ready web application with modern UI

This project proves:
- Deep understanding of algorithms and data structures
- Ability to optimize performance-critical code
- Full-stack development skills (backend + frontend)
- Software engineering best practices
- Cross-platform integration expertise
```

---

## **END OF SPECIFICATION**

```yaml
Document Version: 1.0
Last Updated: 2024-04-24
Total Pages: ~50
Estimated Implementation Time: 35 days (5 weeks)
Difficulty Level: Advanced
Learning Value: ⭐⭐⭐⭐⭐

Next Steps:
  1. Read this document completely
  2. Set up project structure (Phase 1)
  3. Start with DCT implementation (Phase 2, Day 3-4)
  4. Follow roadmap systematically
  5. Track progress with checklist (Section 15)

Good luck building! 🚀
```

---

**This specification is comprehensive, detailed, and designed to prevent hallucination by providing explicit requirements, examples, and constraints. Every decision is documented, every function is specified, and every metric is quantified. An AI agent following this spec will build a production-quality project worthy of a senior engineer's resume.**