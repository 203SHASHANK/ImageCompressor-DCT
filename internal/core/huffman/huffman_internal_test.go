package huffman

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
)

// coefficientLike returns a stream shaped like quantized DCT output: mostly
// zeros and small values, with an end-of-block marker now and then.
func coefficientLike(n int, seed int64) []int {
	rng := rand.New(rand.NewSource(seed))
	out := make([]int, n)
	for i := range out {
		switch r := rng.Intn(100); {
		case r < 55:
			out[i] = 0
		case r < 85:
			out[i] = rng.Intn(5) - 2
		case r < 97:
			out[i] = rng.Intn(41) - 20
		default:
			out[i] = minSymbol
		}
	}
	return out
}

func roundTrip(t *testing.T, in []int) []byte {
	t.Helper()
	encoded, err := NewEncoder(in).Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("decoded %d symbols, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("symbol %d: got %d, want %d", i, out[i], in[i])
		}
	}
	return encoded
}

func TestRoundTripShapes(t *testing.T) {
	cases := map[string][]int{
		"empty":         {},
		"one symbol":    {5},
		"repeated":      {7, 7, 7, 7, 7, 7, 7, 7, 7},
		"two symbols":   {0, 1},
		"range limits":  {minSymbol, maxSymbol, 0, minSymbol, maxSymbol},
		"realistic":     coefficientLike(50000, 1),
		"bit boundary":  {0, 0, 0, 0, 0, 0, 0, 1}, // exactly 8 one-bit codes
		"skewed":        append(make([]int, 1000), 1, 2, 3, 4, 5, 6, 7, 8, 9),
		"every 8 sizes": nil,
	}
	delete(cases, "every 8 sizes")
	for name, in := range cases {
		t.Run(name, func(t *testing.T) { roundTrip(t, in) })
	}
	// Lengths around a byte boundary exercise the padding logic.
	for n := 1; n <= 40; n++ {
		roundTrip(t, coefficientLike(n, int64(n)))
	}
}

// Go randomizes map iteration and heaps do not order ties, so determinism
// has to be designed in. Byte-identical output makes results reproducible.
func TestEncodingIsDeterministic(t *testing.T) {
	in := coefficientLike(20000, 2)
	first := roundTrip(t, in)
	for i := 0; i < 10; i++ {
		if !bytes.Equal(roundTrip(t, in), first) {
			t.Fatal("same input encoded to different bytes")
		}
	}
}

func TestSymbolOutOfRange(t *testing.T) {
	for _, in := range [][]int{{0, maxSymbol + 1}, {minSymbol - 1, 0}} {
		if _, err := NewEncoder(in).Encode(in); !errors.Is(err, ErrSymbolRange) {
			t.Errorf("input %v: got %v, want ErrSymbolRange", in, err)
		}
	}
}

func TestEncodeRejectsUnknownSymbol(t *testing.T) {
	enc := NewEncoder([]int{1, 2, 3, 1, 2})
	if _, err := enc.Encode([]int{1, 2, 99}); err == nil {
		t.Error("expected an error for a symbol the code was not built for")
	}
	single := NewEncoder([]int{4, 4, 4})
	if _, err := single.Encode([]int{4, 5}); err == nil {
		t.Error("expected an error in the single-symbol path")
	}
}

// Frequent symbols must not get longer codes than rare ones.
func TestFrequentSymbolsGetShorterCodes(t *testing.T) {
	in := append(make([]int, 900), make([]int, 0)...)
	for i := 0; i < 90; i++ {
		in = append(in, 1)
	}
	for i := 0; i < 10; i++ {
		in = append(in, 2, 3)
	}
	enc := NewEncoder(in)
	length := func(sym int) uint8 { return enc.codes[sym-enc.base].length }
	if !(length(0) <= length(1) && length(1) <= length(2)) {
		t.Errorf("code lengths not ordered by frequency: 0=%d 1=%d 2=%d", length(0), length(1), length(2))
	}
	if length(0) != 1 {
		t.Errorf("dominant symbol has a %d-bit code, want 1", length(0))
	}
}

func TestDecodeLimit(t *testing.T) {
	in := coefficientLike(1000, 3)
	encoded, _ := NewEncoder(in).Encode(in)
	if _, err := DecodeLimit(encoded, 999); err == nil {
		t.Error("expected an error when output exceeds the limit")
	}
	if _, err := DecodeLimit(encoded, 1000); err != nil {
		t.Errorf("limit equal to the size should pass: %v", err)
	}

	// A 10-byte input that claims four billion symbols must be refused
	// before anything is allocated.
	bomb := []byte{1, 0, 0, 0, 7, 0, 0xff, 0xff, 0xff, 0xff}
	if _, err := Decode(bomb); err == nil {
		t.Error("expected an error for an oversized repeat count")
	}
}

func TestDecodeRejectsMalformedInput(t *testing.T) {
	in := coefficientLike(500, 4)
	valid, _ := NewEncoder(in).Encode(in)

	mutate := func(fn func(b []byte) []byte) []byte {
		return fn(append([]byte(nil), valid...))
	}
	cases := map[string][]byte{
		"empty":             nil,
		"header only":       valid[:6],
		"tree cut short":    valid[:12],
		"unknown node tag":  mutate(func(b []byte) []byte { b[8] = 'Z'; return b }),
		"wrong leaf count":  mutate(func(b []byte) []byte { b[0]++; return b }),
		"tree length lies":  mutate(func(b []byte) []byte { b[4]--; return b }),
		"padding too large": mutate(func(b []byte) []byte { b[8+int(b[4])|int(b[5])<<8] = 9; return b }),
		"single leaf tree":  {2, 0, 0, 0, 3, 0, 0, 0, 'L', 1, 0, 0},
	}
	for name, data := range cases {
		if _, err := Decode(data); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDecodeSurvivesCorruption(t *testing.T) {
	in := coefficientLike(2000, 5)
	valid, _ := NewEncoder(in).Encode(in)
	rng := rand.New(rand.NewSource(6))
	for i := 0; i < 5000; i++ {
		data := append([]byte(nil), valid...)
		data[rng.Intn(len(data))] ^= byte(1 << rng.Intn(8))
		if rng.Intn(3) == 0 {
			data = data[:rng.Intn(len(data))]
		}
		_, _ = DecodeLimit(data, 1<<20)
	}
}

func FuzzDecode(f *testing.F) {
	in := coefficientLike(200, 7)
	valid, _ := NewEncoder(in).Encode(in)
	f.Add(valid)
	f.Add([]byte{1, 0, 0, 0, 7, 0, 3, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeLimit(data, 1<<16)
	})
}

func BenchmarkEncode(b *testing.B) {
	in := coefficientLike(1<<20, 8)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewEncoder(in).Encode(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	in := coefficientLike(1<<20, 8)
	encoded, _ := NewEncoder(in).Encode(in)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(encoded); err != nil {
			b.Fatal(err)
		}
	}
}
