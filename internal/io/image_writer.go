package io

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
)

// OutputFormat specifies the target encoding format for WriteImage.
type OutputFormat string

const (
	FormatJPEG OutputFormat = "jpeg"
	FormatPNG  OutputFormat = "png"
)

// WriteImage encodes an image to disk in the specified format.
func WriteImage(img image.Image, filePath string, format OutputFormat, quality int) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %w", filePath, err)
	}
	defer file.Close()

	return encodeImage(img, file, format, quality)
}

// EncodeImageToBytes encodes an image to a byte slice in the specified format.
func EncodeImageToBytes(img image.Image, format OutputFormat, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeImage(img, &buf, format, quality); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeImage writes an image to an io.Writer in the given format.
func encodeImage(img image.Image, writer io.Writer, format OutputFormat, quality int) error {
	switch strings.ToLower(string(format)) {
	case "jpeg", "jpg":
		if err := jpeg.Encode(writer, img, &jpeg.Options{Quality: clampQuality(quality)}); err != nil {
			return fmt.Errorf("JPEG encoding failed: %w", err)
		}
	case "png":
		encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := encoder.Encode(writer, img); err != nil {
			return fmt.Errorf("PNG encoding failed: %w", err)
		}
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
	return nil
}

// clampQuality ensures quality is in [1, 100].
func clampQuality(quality int) int {
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}
