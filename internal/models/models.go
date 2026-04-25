// Package models defines shared domain types used across the compression pipeline.
package models

import "time"

// CompressionResult holds the output of a single compression operation.
type CompressionResult struct {
	Data             []byte
	OriginalSize     int
	CompressedSize   int
	CompressionRatio float64
	Width            int
	Height           int
	Quality          int
}

// BenchmarkResult holds metrics for one compression method in a benchmark run.
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

// BenchmarkReport aggregates results from all methods for a single image.
type BenchmarkReport struct {
	OriginalSize int
	Quality      int
	Results      []BenchmarkResult
}

// ImageMetadata describes an uploaded image before compression.
type ImageMetadata struct {
	ID        string `json:"id"`
	Format    string `json:"format"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int    `json:"size_bytes"`
	ColorMode string `json:"color_mode"`
}
