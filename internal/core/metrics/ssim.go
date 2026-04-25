package metrics

import (
	"fmt"
	"image"
	"math"
)

// ssimWindowSize is the side length of the sliding window used in SSIM.
const ssimWindowSize = 8

// CalculateSSIM computes the Structural Similarity Index between two images.
// SSIM ranges from 0 to 1, where 1 means identical images.
// Values above 0.9 are generally acceptable; above 0.95 is excellent.
func CalculateSSIM(original, compressed image.Image) (float64, error) {
	if original.Bounds() != compressed.Bounds() {
		return 0, fmt.Errorf("image dimensions mismatch: %v vs %v", original.Bounds(), compressed.Bounds())
	}

	bounds := original.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width < ssimWindowSize || height < ssimWindowSize {
		return 0, fmt.Errorf("image too small for SSIM (minimum %dx%d)", ssimWindowSize, ssimWindowSize)
	}

	// Extract luminance (Y) channel as float64 for both images.
	originalLuma := extractLuminance(original)
	compressedLuma := extractLuminance(compressed)

	// SSIM stability constants (standard values from Wang et al. 2004).
	const k1 = 0.01
	const k2 = 0.03
	const dynamicRange = 255.0
	c1 := (k1 * dynamicRange) * (k1 * dynamicRange)
	c2 := (k2 * dynamicRange) * (k2 * dynamicRange)

	ssimSum := 0.0
	windowCount := 0

	for row := 0; row <= height-ssimWindowSize; row += ssimWindowSize / 2 {
		for col := 0; col <= width-ssimWindowSize; col += ssimWindowSize / 2 {
			meanX, meanY, varX, varY, covarXY := windowStats(
				originalLuma, compressedLuma, row, col, width, height,
			)

			numerator := (2*meanX*meanY + c1) * (2*covarXY + c2)
			denominator := (meanX*meanX + meanY*meanY + c1) * (varX + varY + c2)

			if denominator != 0 {
				ssimSum += numerator / denominator
				windowCount++
			}
		}
	}

	if windowCount == 0 {
		return 0, fmt.Errorf("no valid windows for SSIM calculation")
	}

	return ssimSum / float64(windowCount), nil
}

// extractLuminance converts an image to a 2D float64 luminance (Y) plane.
func extractLuminance(img image.Image) [][]float64 {
	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	luma := make([][]float64, height)
	for row := 0; row < height; row++ {
		luma[row] = make([]float64, width)
		for col := 0; col < width; col++ {
			r, g, b, _ := img.At(bounds.Min.X+col, bounds.Min.Y+row).RGBA()
			rf := float64(r >> 8)
			gf := float64(g >> 8)
			bf := float64(b >> 8)
			luma[row][col] = 0.299*rf + 0.587*gf + 0.114*bf
		}
	}
	return luma
}

// windowStats computes mean, variance, and covariance for an ssimWindowSize×ssimWindowSize
// region starting at (startRow, startCol) in two luminance planes.
func windowStats(planeX, planeY [][]float64, startRow, startCol, width, height int) (meanX, meanY, varX, varY, covarXY float64) {
	count := 0.0

	for row := startRow; row < startRow+ssimWindowSize && row < height; row++ {
		for col := startCol; col < startCol+ssimWindowSize && col < width; col++ {
			meanX += planeX[row][col]
			meanY += planeY[row][col]
			count++
		}
	}

	if count == 0 {
		return
	}

	meanX /= count
	meanY /= count

	for row := startRow; row < startRow+ssimWindowSize && row < height; row++ {
		for col := startCol; col < startCol+ssimWindowSize && col < width; col++ {
			dx := planeX[row][col] - meanX
			dy := planeY[row][col] - meanY
			varX += dx * dx
			varY += dy * dy
			covarXY += dx * dy
		}
	}

	varX /= count
	varY /= count
	covarXY /= count

	return
}

// ensure math is used
var _ = math.Sqrt
