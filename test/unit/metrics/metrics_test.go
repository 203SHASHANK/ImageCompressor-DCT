package metrics_test

import (
	"image"
	"image/color"
	"imagecompressor-dct/internal/core/metrics"
	"math"
	"testing"
)

func TestCalculatePSNR_IdenticalImages(t *testing.T) {
	img := createUniformImage(64, 64, color.RGBA{R: 128, G: 128, B: 128, A: 255})
	psnr, err := metrics.CalculatePSNR(img, img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !math.IsInf(psnr, 1) {
		t.Errorf("expected +Inf for identical images, got %f", psnr)
	}
}

func TestCalculatePSNR_DifferentImages(t *testing.T) {
	original := createUniformImage(64, 64, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	compressed := createUniformImage(64, 64, color.RGBA{R: 250, G: 0, B: 0, A: 255})

	psnr, err := metrics.CalculatePSNR(original, compressed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if psnr <= 0 {
		t.Errorf("PSNR should be positive, got %f", psnr)
	}
	// Small difference → high PSNR.
	if psnr < 30 {
		t.Errorf("expected PSNR > 30 dB for small difference, got %.2f", psnr)
	}
}

func TestCalculatePSNR_DimensionMismatch(t *testing.T) {
	img1 := createUniformImage(64, 64, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	img2 := createUniformImage(32, 32, color.RGBA{R: 100, G: 100, B: 100, A: 255})

	_, err := metrics.CalculatePSNR(img1, img2)
	if err == nil {
		t.Error("expected error for dimension mismatch")
	}
}

func TestCalculateMSE_ZeroForIdentical(t *testing.T) {
	img := createUniformImage(32, 32, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	mse, err := metrics.CalculateMSE(img, img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mse != 0 {
		t.Errorf("MSE should be 0 for identical images, got %f", mse)
	}
}

func TestCalculateSSIM_IdenticalImages(t *testing.T) {
	img := createUniformImage(64, 64, color.RGBA{R: 128, G: 64, B: 200, A: 255})
	ssim, err := metrics.CalculateSSIM(img, img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// SSIM of identical images should be very close to 1.
	if math.Abs(ssim-1.0) > 0.01 {
		t.Errorf("SSIM of identical images should be ~1.0, got %f", ssim)
	}
}

func TestCalculateSSIM_TooSmall(t *testing.T) {
	img := createUniformImage(4, 4, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	_, err := metrics.CalculateSSIM(img, img)
	if err == nil {
		t.Error("expected error for image smaller than SSIM window")
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

func createUniformImage(width, height int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
