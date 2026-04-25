// Package services contains business logic that orchestrates the compression pipeline.
package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/core/metrics"
	"imagecompressor-dct/internal/models"
	"time"
)

// CompressionService orchestrates image compression using the custom DCT encoder.
type CompressionService struct{}

// NewCompressionService creates a CompressionService.
func NewCompressionService() *CompressionService {
	return &CompressionService{}
}

// CompressRequest holds the parameters for a single compression operation.
type CompressRequest struct {
	Image             image.Image
	Quality           int
	ChromaSubsampling dct.ChromaSubsampling
}

// CompressResponse holds the result of a compression operation including metrics.
type CompressResponse struct {
	Result       *models.CompressionResult
	PSNR         float64
	SSIM         float64
	EncodingTime time.Duration
	// DecodedImage is the reconstructed image used for metric calculation.
	DecodedImage image.Image
}

// Compress runs the full DCT compression pipeline and calculates quality metrics.
func (service *CompressionService) Compress(request CompressRequest) (*CompressResponse, error) {
	encoder, err := dct.NewEncoder(dct.CompressionOptions{
		Quality:           request.Quality,
		ChromaSubsampling: request.ChromaSubsampling,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder: %w", err)
	}

	encodeStart := time.Now()
	result, err := encoder.Encode(request.Image)
	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}
	encodingTime := time.Since(encodeStart)

	// Decode the compressed data to measure quality loss.
	decoder := dct.NewDecoder()
	decodedImage, err := decoder.Decode(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding for metric calculation failed: %w", err)
	}

	psnrValue, err := metrics.CalculatePSNR(request.Image, decodedImage)
	if err != nil {
		return nil, fmt.Errorf("PSNR calculation failed: %w", err)
	}

	ssimValue, err := metrics.CalculateSSIM(request.Image, decodedImage)
	if err != nil {
		return nil, fmt.Errorf("SSIM calculation failed: %w", err)
	}

	return &CompressResponse{
		Result:       result,
		PSNR:         psnrValue,
		SSIM:         ssimValue,
		EncodingTime: encodingTime,
		DecodedImage: decodedImage,
	}, nil
}

// CompressGoJPEG compresses an image using Go's stdlib JPEG encoder for benchmarking.
func CompressGoJPEG(img image.Image, quality int) (models.BenchmarkResult, error) {
	start := time.Now()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("Go JPEG encode failed: %w", err)
	}
	encodingTime := time.Since(start)

	// Decode for metrics.
	decodeStart := time.Now()
	decodedImage, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("Go JPEG decode failed: %w", err)
	}
	decodingTime := time.Since(decodeStart)

	originalSize := img.Bounds().Dx() * img.Bounds().Dy() * 3
	compressedSize := buf.Len()

	psnrValue, _ := metrics.CalculatePSNR(img, decodedImage)
	ssimValue, _ := metrics.CalculateSSIM(img, decodedImage)

	return models.BenchmarkResult{
		Method:           "Go stdlib JPEG",
		Language:         "Go",
		CompressedSize:   compressedSize,
		CompressionRatio: float64(originalSize) / float64(compressedSize),
		EncodingTime:     encodingTime,
		DecodingTime:     decodingTime,
		PSNR:             psnrValue,
		SSIM:             ssimValue,
	}, nil
}
