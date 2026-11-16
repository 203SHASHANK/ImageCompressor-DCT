package metrics

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"
)

func noisy(width, height int, seed int64) *image.RGBA {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] =
			byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256)), 255
	}
	return img
}

// slowMSE reads pixels through the generic image interface, the obvious but
// slow way. The optimized implementation must give the same answer.
func slowMSE(a, b image.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	sum := 0.0
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			for _, d := range []float64{
				float64(r1>>8) - float64(r2>>8),
				float64(g1>>8) - float64(g2>>8),
				float64(b1>>8) - float64(b2>>8),
			} {
				sum += d * d
			}
		}
	}
	return sum / float64(ab.Dx()*ab.Dy()*3)
}

func TestMSEMatchesReference(t *testing.T) {
	a, b := noisy(37, 23, 1), noisy(37, 23, 2)
	got, err := CalculateMSE(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if want := slowMSE(a, b); math.Abs(got-want) > 1e-9 {
		t.Errorf("MSE = %v, want %v", got, want)
	}
}

// The same pixels in a different image type, or at a different offset, must
// measure the same.
func TestMetricsIgnoreImageTypeAndOrigin(t *testing.T) {
	a, b := noisy(40, 32, 3), noisy(40, 32, 4)

	nrgba := image.NewNRGBA(a.Bounds())
	for y := 0; y < 32; y++ {
		for x := 0; x < 40; x++ {
			nrgba.Set(x, y, a.At(x, y))
		}
	}
	shifted := image.NewRGBA(image.Rect(100, 50, 140, 82))
	copy(shifted.Pix, b.Pix)

	want, _ := CalculatePSNR(a, b)
	got, err := CalculatePSNR(nrgba, shifted)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("PSNR = %v, want %v", got, want)
	}

	wantSSIM, _ := CalculateSSIM(a, b)
	gotSSIM, err := CalculateSSIM(nrgba, shifted)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(gotSSIM-wantSSIM) > 1e-9 {
		t.Errorf("SSIM = %v, want %v", gotSSIM, wantSSIM)
	}
}

func TestPSNRKnownValue(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 8, 8))
	b := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(a.Pix); i += 4 {
		a.Pix[i], a.Pix[i+1], a.Pix[i+2], a.Pix[i+3] = 100, 100, 100, 255
		b.Pix[i], b.Pix[i+1], b.Pix[i+2], b.Pix[i+3] = 110, 110, 110, 255
	}
	// Every sample is off by 10, so MSE = 100 and PSNR = 10*log10(65025/100).
	got, _ := CalculatePSNR(a, b)
	if want := 10 * math.Log10(650.25); math.Abs(got-want) > 1e-9 {
		t.Errorf("PSNR = %v, want %v", got, want)
	}
}

func TestSSIMOrdering(t *testing.T) {
	base := noisy(64, 64, 5)
	distort := func(amount int) *image.RGBA {
		rng := rand.New(rand.NewSource(9))
		out := image.NewRGBA(base.Bounds())
		for i := range base.Pix {
			v := int(base.Pix[i]) + rng.Intn(2*amount+1) - amount
			out.Pix[i] = byte(math.Max(0, math.Min(255, float64(v))))
		}
		return out
	}
	same, _ := CalculateSSIM(base, base)
	light, _ := CalculateSSIM(base, distort(5))
	heavy, _ := CalculateSSIM(base, distort(60))
	if math.Abs(same-1) > 1e-12 || !(light < same && heavy < light) {
		t.Errorf("SSIM not ordered by distortion: same=%v light=%v heavy=%v", same, light, heavy)
	}
}

func TestSSIMFlatImages(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 16, 16))
	b := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < len(a.Pix); i += 4 {
		a.Set(i/4%16, i/4/16, color.RGBA{0, 0, 0, 255})
		b.Set(i/4%16, i/4/16, color.RGBA{0, 0, 0, 255})
	}
	// Zero mean and zero variance: only the stabilizing constants keep the
	// formula defined.
	got, err := CalculateSSIM(a, b)
	if err != nil || math.IsNaN(got) || math.Abs(got-1) > 1e-12 {
		t.Errorf("SSIM of identical flat images = %v, %v", got, err)
	}
}

func BenchmarkPSNR1080p(b *testing.B) {
	x, y := noisy(1920, 1080, 1), noisy(1920, 1080, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CalculatePSNR(x, y); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSSIM1080p(b *testing.B) {
	x, y := noisy(1920, 1080, 1), noisy(1920, 1080, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CalculateSSIM(x, y); err != nil {
			b.Fatal(err)
		}
	}
}
