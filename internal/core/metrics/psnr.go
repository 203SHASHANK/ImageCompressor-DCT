// Package metrics measures how closely a decoded image matches the original:
// MSE and PSNR on the RGB channels, and SSIM on brightness.
package metrics

import (
	"fmt"
	"image"
	"math"

	"imagecompressor-dct/internal/core/pixels"
)

// CalculateMSE returns the mean squared error between two images of the same
// size, averaged over every R, G and B sample. 0 means identical.
func CalculateMSE(original, compressed image.Image) (float64, error) {
	a, b, err := comparable(original, compressed)
	if err != nil {
		return 0, err
	}

	// Squared differences of 8-bit values are small integers, so the sum is
	// exact in a uint64 and only converted to floating point at the end.
	var sum uint64
	for i := 0; i < len(a.Pix); i += 4 {
		dr := int(a.Pix[i]) - int(b.Pix[i])
		dg := int(a.Pix[i+1]) - int(b.Pix[i+1])
		db := int(a.Pix[i+2]) - int(b.Pix[i+2])
		sum += uint64(dr*dr + dg*dg + db*db)
	}
	samples := len(a.Pix) / 4 * 3
	return float64(sum) / float64(samples), nil
}

// CalculatePSNR returns the peak signal-to-noise ratio in decibels:
//
//	PSNR = 10 * log10(255^2 / MSE)
//
// Higher is better. As a rough guide, lossy photo compression lands between
// 30 and 40 dB, and each 6 dB halves the average error. Identical images
// give +Inf.
func CalculatePSNR(original, compressed image.Image) (float64, error) {
	mse, err := CalculateMSE(original, compressed)
	if err != nil {
		return 0, fmt.Errorf("PSNR: %w", err)
	}
	if mse == 0 {
		return math.Inf(1), nil
	}
	const peak = 255.0
	return 10 * math.Log10(peak*peak/mse), nil
}

// comparable checks that two images have the same, non-zero size and returns
// both in a directly indexable form.
func comparable(original, compressed image.Image) (a, b *image.RGBA, err error) {
	ob, cb := original.Bounds(), compressed.Bounds()
	if ob.Dx() != cb.Dx() || ob.Dy() != cb.Dy() {
		return nil, nil, fmt.Errorf("image dimensions mismatch: %dx%d vs %dx%d",
			ob.Dx(), ob.Dy(), cb.Dx(), cb.Dy())
	}
	if ob.Dx() == 0 || ob.Dy() == 0 {
		return nil, nil, fmt.Errorf("images have zero pixels")
	}
	return pixels.ToRGBA(original), pixels.ToRGBA(compressed), nil
}
