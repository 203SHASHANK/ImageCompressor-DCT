// Package services ties the codec, the quality metrics and the reference
// encoders together for the HTTP layer.
package services

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"time"

	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/core/metrics"
	"imagecompressor-dct/internal/models"
)

// CompressionService runs the codec and measures the result.
type CompressionService struct{}

// NewCompressionService creates a CompressionService.
func NewCompressionService() *CompressionService {
	return &CompressionService{}
}

// CompressRequest holds the parameters of one compression.
type CompressRequest struct {
	Image             image.Image
	Quality           int
	ChromaSubsampling dct.ChromaSubsampling
}

// CompressResponse is the compressed data together with its measurements.
type CompressResponse struct {
	Result       *models.CompressionResult
	PSNR         float64
	SSIM         float64
	EncodingTime time.Duration // encode only
	DecodingTime time.Duration // decode only
	// DecodedImage is the image a reader of the .dct file would see. The
	// quality metrics compare it against the input.
	DecodedImage image.Image
}

// Compress encodes the image, decodes it again and measures how much the
// round trip changed it.
func (s *CompressionService) Compress(request CompressRequest) (*CompressResponse, error) {
	encoder, err := dct.NewEncoder(dct.CompressionOptions{
		Quality:           request.Quality,
		ChromaSubsampling: request.ChromaSubsampling,
	})
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result, err := encoder.Encode(request.Image)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	encodingTime := time.Since(start)

	start = time.Now()
	decoded, err := dct.NewDecoder().Decode(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	decodingTime := time.Since(start)

	psnr, ssim, err := Quality(request.Image, decoded)
	if err != nil {
		return nil, err
	}

	return &CompressResponse{
		Result:       result,
		PSNR:         psnr,
		SSIM:         ssim,
		EncodingTime: encodingTime,
		DecodingTime: decodingTime,
		DecodedImage: decoded,
	}, nil
}

// LosslessPSNR is reported when the decoded image equals the original
// exactly. The true value is infinite, which JSON cannot represent.
const LosslessPSNR = 100.0

// Quality returns PSNR and SSIM of decoded measured against original.
//
// SSIM needs at least an 8x8 image; for anything smaller it is reported as 0
// instead of failing the whole operation.
func Quality(original, decoded image.Image) (psnr, ssim float64, err error) {
	psnr, err = metrics.CalculatePSNR(original, decoded)
	if err != nil {
		return 0, 0, err
	}
	if math.IsInf(psnr, 1) || psnr > LosslessPSNR {
		psnr = LosslessPSNR
	}
	if b := original.Bounds(); b.Dx() >= 8 && b.Dy() >= 8 {
		ssim, err = metrics.CalculateSSIM(original, decoded)
		if err != nil {
			return 0, 0, err
		}
	}
	return psnr, ssim, nil
}

// CompressGoJPEG runs the standard library JPEG encoder on the image and
// measures it the same way, as a baseline for the benchmark.
func CompressGoJPEG(img image.Image, quality int) (models.BenchmarkResult, error) {
	var buf bytes.Buffer

	start := time.Now()
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("JPEG encode: %w", err)
	}
	encodingTime := time.Since(start)

	start = time.Now()
	decoded, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("JPEG decode: %w", err)
	}
	decodingTime := time.Since(start)

	psnr, ssim, err := Quality(img, decoded)
	if err != nil {
		return models.BenchmarkResult{}, err
	}

	return models.BenchmarkResult{
		Method:           "Go stdlib JPEG",
		Language:         "Go",
		CompressedSize:   buf.Len(),
		CompressionRatio: rawRatio(img, buf.Len()),
		EncodingTime:     encodingTime,
		DecodingTime:     decodingTime,
		PSNR:             psnr,
		SSIM:             ssim,
	}, nil
}

// rawRatio is the compression ratio against uncompressed 24-bit RGB, the
// baseline used for every codec in the benchmark.
func rawRatio(img image.Image, compressedSize int) float64 {
	if compressedSize <= 0 {
		return 0
	}
	b := img.Bounds()
	return float64(b.Dx()*b.Dy()*3) / float64(compressedSize)
}
