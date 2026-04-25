// Package metrics provides image quality measurement functions including
// PSNR, SSIM, and MSE for evaluating compression fidelity.
package metrics

import (
	"fmt"
	"image"
	"math"
)

// CalculateMSE computes the Mean Squared Error between two images.
// MSE measures average squared pixel difference across all channels.
// Lower values indicate higher similarity.
func CalculateMSE(original, compressed image.Image) (float64, error) {
	if original.Bounds() != compressed.Bounds() {
		return 0, fmt.Errorf("image dimensions mismatch: %v vs %v", original.Bounds(), compressed.Bounds())
	}

	bounds := original.Bounds()
	sumSquaredError := 0.0
	pixelCount := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, _ := original.At(x, y).RGBA()
			r2, g2, b2, _ := compressed.At(x, y).RGBA()

			// RGBA() returns values in [0, 65535]; shift to [0, 255].
			rf1, gf1, bf1 := float64(r1>>8), float64(g1>>8), float64(b1>>8)
			rf2, gf2, bf2 := float64(r2>>8), float64(g2>>8), float64(b2>>8)

			sumSquaredError += math.Pow(rf1-rf2, 2) + math.Pow(gf1-gf2, 2) + math.Pow(bf1-bf2, 2)
			pixelCount += 3
		}
	}

	if pixelCount == 0 {
		return 0, fmt.Errorf("images have zero pixels")
	}

	return sumSquaredError / float64(pixelCount), nil
}

// CalculatePSNR computes the Peak Signal-to-Noise Ratio between two images.
// PSNR is measured in decibels (dB); higher values indicate better quality.
// Values above 30 dB are generally acceptable; above 40 dB is excellent.
// Returns +Inf if the images are identical.
func CalculatePSNR(original, compressed image.Image) (float64, error) {
	mse, err := CalculateMSE(original, compressed)
	if err != nil {
		return 0, fmt.Errorf("PSNR calculation failed: %w", err)
	}

	if mse == 0 {
		return math.Inf(1), nil
	}

	const maxPixelValue = 255.0
	psnr := 10 * math.Log10((maxPixelValue * maxPixelValue) / mse)
	return psnr, nil
}
