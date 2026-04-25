// Package io provides file I/O utilities for loading and writing images
// in all supported formats (PNG, JPEG, WebP, BMP, TIFF).
package io

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/tiff" // register TIFF decoder
	_ "golang.org/x/image/webp" // register WebP decoder
)

// SupportedFormats lists the file extensions accepted by LoadImage.
var SupportedFormats = []string{".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tiff", ".tif"}

// LoadImage reads an image file from disk and returns it as image.Image.
// Supported formats: PNG, JPEG, WebP, BMP, TIFF.
func LoadImage(filePath string) (image.Image, string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if !isSupportedExtension(ext) {
		return nil, "", fmt.Errorf("unsupported file extension %q; supported: %v", ext, SupportedFormats)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open image file %q: %w", filePath, err)
	}
	defer file.Close()

	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image %q: %w", filePath, err)
	}

	return img, format, nil
}

// LoadImageFromBytes decodes an image from an in-memory byte slice.
func LoadImageFromBytes(data []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image from bytes: %w", err)
	}
	return img, format, nil
}

// isSupportedExtension checks whether the given file extension is supported.
func isSupportedExtension(ext string) bool {
	for _, supported := range SupportedFormats {
		if ext == supported {
			return true
		}
	}
	return false
}
