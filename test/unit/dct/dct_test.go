package dct_test

import (
	"image"
	"image/color"
	"imagecompressor-dct/internal/core/dct"
	"math"
	"testing"
)

// ── Transform tests ────────────────────────────────────────────────────────

func TestForwardDCT2D_ZeroBlock(t *testing.T) {
	var zeroBlock [8][8]float64
	result := dct.ForwardDCT2D(zeroBlock)
	for u := 0; u < 8; u++ {
		for v := 0; v < 8; v++ {
			if result[u][v] != 0 {
				t.Errorf("expected 0 at [%d][%d], got %f", u, v, result[u][v])
			}
		}
	}
}

func TestForwardInverseDCT_RoundTrip(t *testing.T) {
	// A block with a known pattern.
	var original [8][8]float64
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			original[row][col] = float64(row*8+col) - 32.0
		}
	}

	dctBlock := dct.ForwardDCT2D(original)
	reconstructed := dct.InverseDCT2D(dctBlock)

	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			diff := math.Abs(original[row][col] - reconstructed[row][col])
			if diff > 1e-9 {
				t.Errorf("round-trip mismatch at [%d][%d]: original=%.6f reconstructed=%.6f diff=%.2e",
					row, col, original[row][col], reconstructed[row][col], diff)
			}
		}
	}
}

func TestForwardDCT2D_ConstantBlock(t *testing.T) {
	// A constant block should produce energy only in the DC coefficient [0][0].
	var constantBlock [8][8]float64
	const pixelValue = 100.0
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			constantBlock[row][col] = pixelValue
		}
	}

	result := dct.ForwardDCT2D(constantBlock)

	// DC coefficient should be non-zero.
	if result[0][0] == 0 {
		t.Error("DC coefficient should be non-zero for constant block")
	}

	// All AC coefficients should be near zero.
	for u := 0; u < 8; u++ {
		for v := 0; v < 8; v++ {
			if u == 0 && v == 0 {
				continue
			}
			if math.Abs(result[u][v]) > 1e-9 {
				t.Errorf("AC coefficient [%d][%d] should be ~0 for constant block, got %f", u, v, result[u][v])
			}
		}
	}
}

// ── Encoder / Decoder round-trip ───────────────────────────────────────────

func TestEncoderDecoder_RoundTrip(t *testing.T) {
	img := createSolidColorImage(64, 64, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	encoder, err := dct.NewEncoder(dct.CompressionOptions{
		Quality:           90,
		ChromaSubsampling: dct.Subsampling444,
	})
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	result, err := encoder.Encode(img)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if result.CompressedSize == 0 {
		t.Error("compressed size should be > 0")
	}
	if result.CompressionRatio <= 0 {
		t.Error("compression ratio should be > 0")
	}

	decoder := dct.NewDecoder()
	decoded, err := decoder.Decode(result.Data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Bounds().Dx() != 64 || decoded.Bounds().Dy() != 64 {
		t.Errorf("decoded dimensions mismatch: got %v", decoded.Bounds())
	}
}

func TestNewEncoder_InvalidQuality(t *testing.T) {
	_, err := dct.NewEncoder(dct.CompressionOptions{Quality: 0})
	if err == nil {
		t.Error("expected error for quality=0")
	}

	_, err = dct.NewEncoder(dct.CompressionOptions{Quality: 101})
	if err == nil {
		t.Error("expected error for quality=101")
	}
}

func TestEncoder_ChromaSubsampling420(t *testing.T) {
	img := createSolidColorImage(32, 32, color.RGBA{R: 128, G: 64, B: 200, A: 255})

	encoder, err := dct.NewEncoder(dct.CompressionOptions{
		Quality:           75,
		ChromaSubsampling: dct.Subsampling420,
	})
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	result, err := encoder.Encode(img)
	if err != nil {
		t.Fatalf("Encode with 4:2:0 failed: %v", err)
	}

	decoder := dct.NewDecoder()
	_, err = decoder.Decode(result.Data)
	if err != nil {
		t.Fatalf("Decode after 4:2:0 encode failed: %v", err)
	}
}

// ── Benchmark ──────────────────────────────────────────────────────────────

func BenchmarkForwardDCT2D(b *testing.B) {
	var block [8][8]float64
	for row := 0; row < 8; row++ {
		for col := 0; col < 8; col++ {
			block[row][col] = float64(row*col) - 32.0
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dct.ForwardDCT2D(block)
	}
}

func BenchmarkEncode1080p(b *testing.B) {
	img := createSolidColorImage(1920, 1080, color.RGBA{R: 100, G: 150, B: 200, A: 255})
	encoder, _ := dct.NewEncoder(dct.CompressionOptions{Quality: 75, ChromaSubsampling: dct.Subsampling420})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Encode(img)
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

func createSolidColorImage(width, height int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
