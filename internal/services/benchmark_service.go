// Package services contains business logic that orchestrates the compression pipeline.
package services

import (
	"fmt"
	"image"
	"image/png"
	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/models"
	"imagecompressor-dct/internal/python"
	"log"
	"os"
)

// BenchmarkService coordinates compression benchmarks across all supported methods.
type BenchmarkService struct {
	compressionService *CompressionService
	pythonBridge       *python.Bridge
}

// NewBenchmarkService creates a BenchmarkService with all available method runners.
func NewBenchmarkService() *BenchmarkService {
	return &BenchmarkService{
		compressionService: NewCompressionService(),
		pythonBridge:       python.NewBridge(),
	}
}

// RunAll compresses img with every available method at the given quality and
// returns all results. Python-based methods are non-fatal: if Python is
// unavailable or a script fails, those results are simply omitted.
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

	// 3 & 4. Python PIL + OpenCV (optional — skipped if Python is unavailable).
	if bs.pythonBridge.IsAvailable() {
		tempPath, cleanup, err := saveTempPNG(img)
		if err != nil {
			log.Printf("Failed to create temp PNG for Python benchmarks: %v", err)
		} else {
			defer cleanup()

			if result, err := bs.pythonBridge.RunPILBenchmark(tempPath, quality); err == nil {
				results = append(results, result)
			} else {
				log.Printf("Python PIL benchmark failed: %v", err)
			}

			if result, err := bs.pythonBridge.RunOpenCVBenchmark(tempPath, quality); err == nil {
				results = append(results, result)
			} else {
				log.Printf("Python OpenCV benchmark failed: %v", err)
			}
		}
	}

	return results
}

// runCustomDCT compresses img with the custom DCT encoder and returns metrics.
func (bs *BenchmarkService) runCustomDCT(img image.Image, quality int) (models.BenchmarkResult, error) {
	response, err := bs.compressionService.Compress(CompressRequest{
		Image:             img,
		Quality:           quality,
		ChromaSubsampling: dct.Subsampling420,
	})
	if err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("custom DCT: %w", err)
	}
	return models.BenchmarkResult{
		Method:           "DCTPress",
		Language:         "Go",
		CompressedSize:   response.Result.CompressedSize,
		CompressionRatio: response.Result.CompressionRatio,
		EncodingTime:     response.EncodingTime,
		PSNR:             response.PSNR,
		SSIM:             response.SSIM,
	}, nil
}

// saveTempPNG writes img to a temporary PNG file and returns the path along
// with a cleanup function that deletes the file.
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
