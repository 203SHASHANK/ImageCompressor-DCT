package dct_test

import (
	"image"
	"image/color"
	"imagecompressor-dct/internal/core/dct"
	"math"
	"testing"
)

func pixelPSNR(orig, dec image.Image) float64 {
	b := orig.Bounds()
	mse := 0.0
	n := float64(b.Dx() * b.Dy() * 3)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			or, og, ob, _ := orig.At(x, y).RGBA()
			dr, dg, db, _ := dec.At(x, y).RGBA()
			d := func(a, b uint32) float64 { v := float64(a>>8) - float64(b>>8); return v * v }
			mse += d(or, dr) + d(og, dg) + d(ob, db)
		}
	}
	mse /= n
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

func makeSinusoidalImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			r := 128 + 80*math.Sin(fx*0.05)*math.Cos(fy*0.03)
			g := 128 + 60*math.Sin(fx*0.07+1.0)*math.Sin(fy*0.05)
			b := 100 + 70*math.Cos(fx*0.03)*math.Cos(fy*0.07+0.5)
			clamp := func(v float64) uint8 {
				if v < 0 {
					return 0
				}
				if v > 255 {
					return 255
				}
				return uint8(v)
			}
			img.SetRGBA(x, y, color.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 255})
		}
	}
	return img
}

// TestRoundTrip_Gradient checks a smooth gradient (mostly low-frequency DCT content).
func TestRoundTrip_Gradient(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / 64),
				G: uint8((y * 255) / 64),
				B: uint8(((x + y) * 128) / 128),
				A: 255,
			})
		}
	}
	enc, _ := dct.NewEncoder(dct.CompressionOptions{Quality: 75, ChromaSubsampling: dct.Subsampling420})
	result, _ := enc.Encode(img)
	decoded, err := dct.NewDecoder().Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	psnr := pixelPSNR(img, decoded)
	t.Logf("Q75 4:2:0 gradient 64×64: PSNR=%.2f dB  compressed=%d bytes", psnr, result.CompressedSize)
	if psnr < 35 {
		t.Errorf("PSNR %.2f dB too low for gradient image", psnr)
	}
}

// TestRoundTrip_Sinusoidal exercises a natural-image-like frequency distribution on a large frame.
func TestRoundTrip_Sinusoidal(t *testing.T) {
	img := makeSinusoidalImage(320, 240)
	rawSize := 320 * 240 * 3
	for _, q := range []int{50, 75, 90} {
		enc, _ := dct.NewEncoder(dct.CompressionOptions{Quality: q, ChromaSubsampling: dct.Subsampling420})
		result, _ := enc.Encode(img)
		decoded, err := dct.NewDecoder().Decode(result.Data)
		if err != nil {
			t.Errorf("Q%d decode error: %v", q, err)
			continue
		}
		psnr := pixelPSNR(img, decoded)
		t.Logf("Q%d sinusoidal 320×240: PSNR=%.2f dB  %d→%d bytes (%.1fx)",
			q, psnr, rawSize, result.CompressedSize, float64(rawSize)/float64(result.CompressedSize))
		if psnr < 28 {
			t.Errorf("Q%d: PSNR %.2f dB too low for sinusoidal image", q, psnr)
		}
	}
}

// TestRoundTrip_EOBEdgeCase verifies that a block where all 63 AC slots are filled does not
// corrupt subsequent blocks (the EOB marker must be consumed even after a full block).
func TestRoundTrip_EOBEdgeCase(t *testing.T) {
	// Q100 encodes with minimal quantization, maximising surviving ACs.
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			v := uint8(((x+1)*(y+1)*7) % 256)
			img.SetRGBA(x, y, color.RGBA{R: v, G: 255 - v, B: v / 2, A: 255})
		}
	}
	enc, _ := dct.NewEncoder(dct.CompressionOptions{Quality: 100, ChromaSubsampling: dct.Subsampling444})
	result, _ := enc.Encode(img)
	decoded, err := dct.NewDecoder().Decode(result.Data)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	psnr := pixelPSNR(img, decoded)
	t.Logf("Q100 16×16 high-freq: PSNR=%.2f dB", psnr)
	if psnr < 40 {
		t.Errorf("Q100 round-trip PSNR too low: %.2f dB — EOB alignment broken?", psnr)
	}
}
