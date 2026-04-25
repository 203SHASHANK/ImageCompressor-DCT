package huffman_test

import (
	"imagecompressor-dct/internal/core/huffman"
	"testing"
)

func TestHuffman_RoundTrip_SimpleSequence(t *testing.T) {
	coefficients := []int{0, 0, 1, -1, 0, 2, 0, 0, 3, -2, 0, 0, 0, 1}

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := huffman.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded) != len(coefficients) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(coefficients))
	}
	for i, value := range coefficients {
		if decoded[i] != value {
			t.Errorf("mismatch at index %d: got %d, want %d", i, decoded[i], value)
		}
	}
}

func TestHuffman_RoundTrip_AllZeros(t *testing.T) {
	coefficients := make([]int, 64)

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := huffman.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	for i, value := range decoded {
		if value != 0 {
			t.Errorf("expected 0 at index %d, got %d", i, value)
		}
	}
}

func TestHuffman_RoundTrip_SingleSymbol(t *testing.T) {
	coefficients := []int{42, 42, 42, 42, 42}

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := huffman.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	for i, value := range decoded {
		if value != 42 {
			t.Errorf("expected 42 at index %d, got %d", i, value)
		}
	}
}

func TestHuffman_RoundTrip_NegativeCoefficients(t *testing.T) {
	coefficients := []int{-100, 50, -25, 12, -6, 3, -1, 0, 0, 0, 1, -1}

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := huffman.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	for i, value := range coefficients {
		if decoded[i] != value {
			t.Errorf("mismatch at index %d: got %d, want %d", i, decoded[i], value)
		}
	}
}

func TestHuffman_RoundTrip_LargeBlock(t *testing.T) {
	// Simulate a realistic DCT coefficient stream: mostly zeros with sparse non-zeros.
	coefficients := make([]int, 640)
	coefficients[0] = 512
	coefficients[1] = -30
	coefficients[8] = 15
	coefficients[64] = 480
	coefficients[65] = -20
	coefficients[128] = 500

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := huffman.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded) != len(coefficients) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(coefficients))
	}
	for i, value := range coefficients {
		if decoded[i] != value {
			t.Errorf("mismatch at index %d: got %d, want %d", i, decoded[i], value)
		}
	}
}

func TestHuffman_CompressionRatio(t *testing.T) {
	// Sparse coefficients (mostly zeros) should compress well vs raw int16.
	coefficients := make([]int, 640)
	coefficients[0] = 100
	coefficients[64] = 80

	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	rawSize := len(coefficients) * 2
	if len(encoded) >= rawSize {
		t.Errorf("Huffman should be smaller than raw int16: encoded=%d raw=%d", len(encoded), rawSize)
	}
}

func BenchmarkHuffmanEncode(b *testing.B) {
	coefficients := make([]int, 64*1000)
	for i := range coefficients {
		if i%64 == 0 {
			coefficients[i] = 200
		}
	}
	encoder := huffman.NewEncoder(coefficients)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.Encode(coefficients)
	}
}
