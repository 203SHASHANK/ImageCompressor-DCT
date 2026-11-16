package dct

import "math"

// The .dct container:
//
//	offset  size  field
//	0       4     magic "DCT\x02"
//	4       1     format version (2)
//	5       4     image width, little-endian
//	9       4     image height, little-endian
//	13      1     quality (1-100)
//	14      1     chroma subsampling (0 = 4:4:4, 1 = 4:2:2, 2 = 4:2:0)
//	15      4     byte length of the Y stream
//	19      4     byte length of the Cb stream
//	23      4     byte length of the Cr stream
//	27      ...   Y, Cb and Cr streams, each Huffman-coded on its own
const (
	headerSize    = 27
	formatVersion = 2

	// maxDimension and maxPixels bound what the decoder will allocate for a
	// file it has not produced itself.
	maxDimension = 65535
	maxPixels    = 1 << 27
)

var magic = [4]byte{'D', 'C', 'T', 0x02}

// eobMarker ("end of block") follows the last non-zero coefficient of every
// block in the coefficient stream and stands for "the rest of this block is
// zero". Quantized coefficients stay within about +/-2100, so this value can
// never be mistaken for one.
const eobMarker = math.MinInt16

// zigzag[i] is the position, as row*8+col, of the i-th coefficient in zigzag
// order. The order walks the block diagonally from the top-left (lowest
// frequency) to the bottom-right (highest), so the coefficients most likely
// to be zero after quantization end up together at the tail.
var zigzag = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

// ChromaSubsampling selects how much the color channels are downscaled.
type ChromaSubsampling int

const (
	// Subsampling444 keeps color at full resolution.
	Subsampling444 ChromaSubsampling = iota
	// Subsampling422 halves the horizontal color resolution.
	Subsampling422
	// Subsampling420 halves the color resolution in both directions.
	Subsampling420
)

func (s ChromaSubsampling) valid() bool {
	return s >= Subsampling444 && s <= Subsampling420
}

// chromaDimensions returns the size of the color planes for an image of the
// given size.
func chromaDimensions(width, height int, s ChromaSubsampling) (int, int) {
	switch s {
	case Subsampling420:
		return ceilDiv(width, 2), ceilDiv(height, 2)
	case Subsampling422:
		return ceilDiv(width, 2), height
	default:
		return width, height
	}
}
