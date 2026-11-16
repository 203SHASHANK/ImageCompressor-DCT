package services

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"

	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/models"
	"imagecompressor-dct/internal/python"
)

// BenchmarkService runs the same image through every available codec.
type BenchmarkService struct {
	compression *CompressionService
	bridge      *python.Bridge
}

// NewBenchmarkService creates a BenchmarkService.
func NewBenchmarkService() *BenchmarkService {
	return &BenchmarkService{
		compression: NewCompressionService(),
		bridge:      python.NewBridge(),
	}
}

// RunAll compresses img with every codec at the given quality.
//
// All codecs are measured identically: size against uncompressed RGB, and
// PSNR/SSIM computed by this program from each codec's own decoded output.
// The Python encoders are optional: if the interpreter or a library is
// missing, that row is logged and left out.
func (bs *BenchmarkService) RunAll(ctx context.Context, img image.Image, quality int) []models.BenchmarkResult {
	var results []models.BenchmarkResult

	if result, err := bs.runCustomDCT(img, quality); err == nil {
		results = append(results, result)
	} else {
		log.Printf("benchmark: DCTPress: %v", err)
	}

	if result, err := CompressGoJPEG(img, quality); err == nil {
		results = append(results, result)
	} else {
		log.Printf("benchmark: Go stdlib JPEG: %v", err)
	}

	if !bs.bridge.IsAvailable() {
		return results
	}

	// The scripts read their input from disk. PNG is lossless, so they see
	// exactly the pixels the Go encoders were given.
	inputPath, cleanup, err := saveTempPNG(img)
	if err != nil {
		log.Printf("benchmark: %v", err)
		return results
	}
	defer cleanup()

	for _, script := range []string{python.ScriptPIL, python.ScriptOpenCV} {
		if ctx.Err() != nil {
			break // the client went away; do not start more work
		}
		result, err := bs.runPython(ctx, script, inputPath, img, quality)
		if err != nil {
			log.Printf("benchmark: %v", err)
			continue
		}
		results = append(results, result)
	}
	return results
}

func (bs *BenchmarkService) runCustomDCT(img image.Image, quality int) (models.BenchmarkResult, error) {
	response, err := bs.compression.Compress(CompressRequest{
		Image:             img,
		Quality:           quality,
		ChromaSubsampling: dct.Subsampling420, // what JPEG encoders use by default
	})
	if err != nil {
		return models.BenchmarkResult{}, err
	}
	return models.BenchmarkResult{
		Method:           "DCTPress",
		Language:         "Go",
		CompressedSize:   response.Result.CompressedSize,
		CompressionRatio: response.Result.CompressionRatio,
		EncodingTime:     response.EncodingTime,
		DecodingTime:     response.DecodingTime,
		PSNR:             response.PSNR,
		SSIM:             response.SSIM,
	}, nil
}

func (bs *BenchmarkService) runPython(ctx context.Context, script, inputPath string, original image.Image, quality int) (models.BenchmarkResult, error) {
	run, err := bs.bridge.Run(ctx, script, inputPath, quality)
	if err != nil {
		return models.BenchmarkResult{}, err
	}
	psnr, ssim, err := Quality(original, run.Decoded)
	if err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("%s: %w", script, err)
	}
	return models.BenchmarkResult{
		Method:           run.Method,
		Language:         run.Language,
		CompressedSize:   run.CompressedSize,
		CompressionRatio: rawRatio(original, run.CompressedSize),
		EncodingTime:     run.EncodingTime,
		DecodingTime:     run.DecodingTime,
		PSNR:             psnr,
		SSIM:             ssim,
	}, nil
}

// saveTempPNG writes img to a temporary PNG file and returns its path and a
// function that removes it.
func saveTempPNG(img image.Image) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "imgcomp-bench-*.png")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	remove := func() { os.Remove(f.Name()) }

	// Fast compression: the file is read once and deleted.
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(f, img); err != nil {
		f.Close()
		remove()
		return "", nil, fmt.Errorf("write temp PNG: %w", err)
	}
	if err := f.Close(); err != nil {
		remove()
		return "", nil, fmt.Errorf("write temp PNG: %w", err)
	}
	return f.Name(), remove, nil
}
