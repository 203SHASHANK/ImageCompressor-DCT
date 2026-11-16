// Package models defines the data types shared between the codec, the
// services and the HTTP layer.
package models

import "time"

// CompressionResult is the output of one encode.
type CompressionResult struct {
	Data             []byte  // the .dct file contents
	OriginalSize     int     // size as uncompressed 24-bit RGB, in bytes
	CompressedSize   int     // len(Data)
	CompressionRatio float64 // OriginalSize / CompressedSize
	Width            int
	Height           int
	Quality          int
}

// BenchmarkResult holds the measurements for one codec in a benchmark run.
//
// It is serialized as-is by the benchmark endpoint, so the JSON keys are the
// Go field names and the two durations are in nanoseconds. The web UI
// depends on that shape.
type BenchmarkResult struct {
	Method           string
	Language         string
	CompressedSize   int
	CompressionRatio float64 // against uncompressed 24-bit RGB
	EncodingTime     time.Duration
	DecodingTime     time.Duration
	PSNR             float64
	SSIM             float64
}

// ImageMetadata describes an uploaded image.
type ImageMetadata struct {
	ID        string `json:"id"`
	Format    string `json:"format"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int    `json:"size_bytes"`
	ColorMode string `json:"color_mode"`
}
