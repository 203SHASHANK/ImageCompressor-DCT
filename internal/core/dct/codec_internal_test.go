package dct

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"runtime"
	"testing"
)

// photoLike builds a deterministic test image with the ingredients of a
// photograph: smooth gradients, a few hard edges and some sensor-style noise.
// A flat or purely synthetic image would flatter the codec.
func photoLike(width, height int) *image.RGBA {
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fx, fy := float64(x)/float64(width), float64(y)/float64(height)
			r := 120 + 90*math.Sin(6*fx)*math.Cos(4*fy)
			g := 110 + 80*math.Sin(3*fx+2*fy)
			b := 100 + 100*fy
			if (x/96+y/96)%5 == 0 { // blocks with hard edges
				r, g, b = r*0.5, g*0.6, b*0.4+60
			}
			noise := rng.NormFloat64() * 4
			img.SetRGBA(x, y, color.RGBA{
				R: clampToByte(r + noise), G: clampToByte(g + noise), B: clampToByte(b + noise), A: 255,
			})
		}
	}
	return img
}

func mustEncode(t testing.TB, img image.Image, quality int, s ChromaSubsampling) []byte {
	t.Helper()
	enc, err := NewEncoder(CompressionOptions{Quality: quality, ChromaSubsampling: s})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	result, err := enc.Encode(img)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return result.Data
}

func psnr(a, b *image.RGBA) float64 {
	var sum float64
	for i := 0; i < len(a.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			d := float64(a.Pix[i+c]) - float64(b.Pix[i+c])
			sum += d * d
		}
	}
	mse := sum / float64(len(a.Pix)/4*3)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

// The separable transform must agree with the textbook double sum.
func TestFastDCTMatchesDefinition(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 50; trial++ {
		var in block
		for r := range in {
			for c := range in[r] {
				in[r][c] = rng.Float64()*255 - 128
			}
		}
		fast, naive := ForwardDCT2D(in), naiveForwardDCT2D(in)
		for r := range fast {
			for c := range fast[r] {
				if math.Abs(fast[r][c]-naive[r][c]) > 1e-9 {
					t.Fatalf("trial %d [%d][%d]: fast=%v naive=%v", trial, r, c, fast[r][c], naive[r][c])
				}
			}
		}
	}
}

func TestZigzagIsAPermutation(t *testing.T) {
	var seen [64]bool
	for _, pos := range zigzag {
		if pos < 0 || pos > 63 || seen[pos] {
			t.Fatalf("zigzag table is not a permutation of 0..63 (problem at %d)", pos)
		}
		seen[pos] = true
	}
	// Spot-check the start and end of the standard order.
	if zigzag[0] != 0 || zigzag[1] != 1 || zigzag[2] != 8 || zigzag[63] != 63 {
		t.Errorf("unexpected zigzag order: %v", zigzag)
	}
}

func TestQuantizationTableQuality(t *testing.T) {
	q50 := buildQuantizationTable(standardLuminanceTable, 50)
	if q50[0][1] != standardLuminanceTable[0][1] {
		t.Errorf("quality 50 should leave AC steps unchanged, got %v", q50[0][1])
	}
	if want := math.Round(standardLuminanceTable[0][0] * dcStepScale); q50[0][0] != want {
		t.Errorf("DC step = %v, want %v", q50[0][0], want)
	}
	q100 := buildQuantizationTable(standardLuminanceTable, 100)
	q10 := buildQuantizationTable(standardLuminanceTable, 10)
	for u := range q100 {
		for v := range q100[u] {
			if q100[u][v] != 1 {
				t.Fatalf("quality 100 step [%d][%d] = %v, want 1", u, v, q100[u][v])
			}
			if q10[u][v] < q50[u][v] || q10[u][v] > 255 {
				t.Fatalf("quality 10 step [%d][%d] = %v out of order", u, v, q10[u][v])
			}
		}
	}
}

// Sizes that are not multiples of 8 (or of 16 with subsampled chroma)
// exercise the edge padding in the encoder and the cropping in the decoder.
func TestRoundTripAwkwardSizes(t *testing.T) {
	sizes := [][2]int{{1, 1}, {1, 9}, {7, 3}, {8, 8}, {9, 17}, {33, 15}, {100, 75}}
	modes := []ChromaSubsampling{Subsampling444, Subsampling422, Subsampling420}

	for _, size := range sizes {
		for _, mode := range modes {
			src := photoLike(size[0], size[1])
			decoded, err := NewDecoder().Decode(mustEncode(t, src, 90, mode))
			if err != nil {
				t.Fatalf("%dx%d mode %d: %v", size[0], size[1], mode, err)
			}
			if decoded.Bounds() != src.Bounds() {
				t.Fatalf("%dx%d mode %d: decoded bounds %v", size[0], size[1], mode, decoded.Bounds())
			}
			// Tiny images are dominated by edge effects; just require sanity.
			if got := psnr(src, decoded.(*image.RGBA)); got < 20 {
				t.Errorf("%dx%d mode %d: PSNR %.1f dB", size[0], size[1], mode, got)
			}
		}
	}
}

// Images whose bounds do not start at (0, 0), and non-RGBA image types, must
// encode the same pixels.
func TestEncodeSubImageAndOtherTypes(t *testing.T) {
	base := photoLike(96, 96)
	sub := base.SubImage(image.Rect(16, 24, 80, 88))

	copied := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			copied.Set(x, y, base.At(x+16, y+24))
		}
	}

	a := mustEncode(t, sub, 80, Subsampling420)
	b := mustEncode(t, copied, 80, Subsampling420)
	if string(a) != string(b) {
		t.Error("sub-image and equivalent NRGBA image encoded differently")
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	img := photoLike(200, 120)
	first := mustEncode(t, img, 75, Subsampling420)
	for i := 0; i < 5; i++ {
		if string(mustEncode(t, img, 75, Subsampling420)) != string(first) {
			t.Fatal("encoding the same image twice gave different bytes")
		}
	}
}

func TestQualityIsMonotonic(t *testing.T) {
	img := photoLike(256, 256)
	prevSize, prevPSNR := 0, 0.0
	for _, q := range []int{10, 30, 50, 75, 90} {
		data := mustEncode(t, img, q, Subsampling420)
		decoded, err := NewDecoder().Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		p := psnr(img, decoded.(*image.RGBA))
		if len(data) <= prevSize || p <= prevPSNR {
			t.Errorf("quality %d: size %d (prev %d), PSNR %.2f (prev %.2f)", q, len(data), prevSize, p, prevPSNR)
		}
		prevSize, prevPSNR = len(data), p
	}
}

func TestNewEncoderRejectsBadSubsampling(t *testing.T) {
	if _, err := NewEncoder(CompressionOptions{Quality: 50, ChromaSubsampling: 7}); err == nil {
		t.Error("expected an error for an unknown subsampling mode")
	}
}

func TestEncodeRejectsEmptyImage(t *testing.T) {
	enc, _ := NewEncoder(CompressionOptions{Quality: 50})
	if _, err := enc.Encode(image.NewRGBA(image.Rect(0, 0, 0, 10))); err == nil {
		t.Error("expected an error for a zero-width image")
	}
}

func TestDecodeRejectsBadHeaders(t *testing.T) {
	valid := mustEncode(t, photoLike(32, 32), 75, Subsampling420)

	mutate := func(fn func(b []byte)) []byte {
		b := append([]byte(nil), valid...)
		fn(b)
		return b
	}
	cases := map[string][]byte{
		"empty":            nil,
		"short":            valid[:10],
		"bad magic":        mutate(func(b []byte) { b[0] = 'X' }),
		"bad version":      mutate(func(b []byte) { b[4] = 9 }),
		"zero width":       mutate(func(b []byte) { b[5], b[6], b[7], b[8] = 0, 0, 0, 0 }),
		"huge width":       mutate(func(b []byte) { b[5], b[6], b[7], b[8] = 0xff, 0xff, 0xff, 0x7f }),
		"too many pixels":  mutate(func(b []byte) { b[5], b[6] = 0xff, 0xff; b[9], b[10] = 0xff, 0xff }),
		"zero quality":     mutate(func(b []byte) { b[13] = 0 }),
		"bad subsampling":  mutate(func(b []byte) { b[14] = 5 }),
		"stream too long":  mutate(func(b []byte) { b[15], b[16], b[17], b[18] = 0xff, 0xff, 0xff, 0xff }),
		"truncated body":   valid[:len(valid)-5],
		"wrong dimensions": mutate(func(b []byte) { b[5] = 64 }), // streams hold fewer blocks than claimed
	}
	for name, data := range cases {
		if _, err := NewDecoder().Decode(data); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// Whatever bytes arrive, Decode must return (possibly with an error) rather
// than panic or hang.
func TestDecodeSurvivesCorruption(t *testing.T) {
	valid := mustEncode(t, photoLike(40, 24), 60, Subsampling420)
	rng := rand.New(rand.NewSource(3))

	for i := 0; i < 3000; i++ {
		data := append([]byte(nil), valid...)
		for flips := 1 + rng.Intn(3); flips > 0; flips-- {
			data[headerSize+rng.Intn(len(data)-headerSize)] ^= byte(1 << rng.Intn(8))
		}
		if rng.Intn(4) == 0 {
			data = data[:headerSize+rng.Intn(len(data)-headerSize)]
		}
		_, _ = NewDecoder().Decode(data)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(mustEncode(f, photoLike(16, 16), 75, Subsampling420))
	f.Add(mustEncode(f, photoLike(9, 5), 50, Subsampling444))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = NewDecoder().Decode(data)
	})
}

func TestUpsampleConstantPlane(t *testing.T) {
	src := newPlane(5, 3)
	for i := range src.samples {
		src.samples[i] = 42
	}
	dst := upsample(src, 10, 6)
	for i, v := range dst.samples {
		if math.Abs(v-42) > 1e-12 {
			t.Fatalf("sample %d = %v, want 42", i, v)
		}
	}
}

func TestDownsampleAverages(t *testing.T) {
	src := newPlane(3, 2)
	copy(src.samples, []float64{1, 3, 10, 5, 7, 20})
	dst := downsample(src, 2, 2)
	if dst.width != 2 || dst.height != 1 {
		t.Fatalf("size %dx%d, want 2x1", dst.width, dst.height)
	}
	if dst.samples[0] != 4 || dst.samples[1] != 15 {
		t.Errorf("got %v, want [4 15]", dst.samples)
	}
}

func TestForEachRowVisitsEveryRowOnce(t *testing.T) {
	for _, n := range []int{0, 1, 2, 7, 1000} {
		visits := make([]int32, n)
		forEachRow(n, func(r int) { visits[r]++ })
		for r, v := range visits {
			if v != 1 {
				t.Fatalf("n=%d: row %d visited %d times", n, r, v)
			}
		}
	}
}

func BenchmarkNaiveDCT2D(b *testing.B) {
	in := benchBlock()
	for i := 0; i < b.N; i++ {
		naiveForwardDCT2D(in)
	}
}

func BenchmarkFastDCT2D(b *testing.B) {
	in := benchBlock()
	for i := 0; i < b.N; i++ {
		ForwardDCT2D(in)
	}
}

func benchBlock() block {
	var in block
	for r := range in {
		for c := range in[r] {
			in[r][c] = float64(r*c) - 32
		}
	}
	return in
}

func BenchmarkEncodePhoto1080p(b *testing.B) {
	benchEncode(b, runtime.GOMAXPROCS(0))
}

// The same encode restricted to one core, to show what the worker pool buys.
func BenchmarkEncodePhoto1080pSingleCore(b *testing.B) {
	benchEncode(b, 1)
}

func benchEncode(b *testing.B, procs int) {
	img := photoLike(1920, 1080)
	enc, _ := NewEncoder(CompressionOptions{Quality: 75, ChromaSubsampling: Subsampling420})
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.Encode(img); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodePhoto1080p(b *testing.B) {
	data := mustEncode(b, photoLike(1920, 1080), 75, Subsampling420)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewDecoder().Decode(data); err != nil {
			b.Fatal(err)
		}
	}
}
