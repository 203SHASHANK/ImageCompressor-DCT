// Package dct implements a JPEG-style lossy image codec: color conversion,
// chroma subsampling, 8x8 block DCT, quantization, zigzag ordering and
// Huffman entropy coding, plus the matching decoder.
package dct

import "math"

const blockSize = 8

// block is one 8x8 tile of samples or of DCT coefficients.
type block = [blockSize][blockSize]float64

// basis[k][n] = 0.5 * alpha(k) * cos((2n+1) * k * pi / 16), with
// alpha(0) = 1/sqrt(2) and alpha(k) = 1 otherwise.
//
// These 64 values are everything the 8-point DCT needs. Computing them once
// replaces a math.Cos call per multiplication in the inner loop with a table
// lookup, and folding the scale factors in removes a branch per term.
var basis [blockSize][blockSize]float64

func init() {
	for k := 0; k < blockSize; k++ {
		alpha := 1.0
		if k == 0 {
			alpha = 1 / math.Sqrt2
		}
		for n := 0; n < blockSize; n++ {
			basis[k][n] = 0.5 * alpha * math.Cos(float64(2*n+1)*float64(k)*math.Pi/16)
		}
	}
}

// forwardDCT1D computes the orthonormal 8-point DCT-II:
//
//	out[k] = sum over n of in[n] * basis[k][n]
func forwardDCT1D(in *[blockSize]float64, out *[blockSize]float64) {
	for k := 0; k < blockSize; k++ {
		row := &basis[k]
		out[k] = in[0]*row[0] + in[1]*row[1] + in[2]*row[2] + in[3]*row[3] +
			in[4]*row[4] + in[5]*row[5] + in[6]*row[6] + in[7]*row[7]
	}
}

// inverseDCT1D computes the inverse of forwardDCT1D (a DCT-III):
//
//	out[n] = sum over k of in[k] * basis[k][n]
func inverseDCT1D(in *[blockSize]float64, out *[blockSize]float64) {
	for n := 0; n < blockSize; n++ {
		out[n] = in[0]*basis[0][n] + in[1]*basis[1][n] + in[2]*basis[2][n] + in[3]*basis[3][n] +
			in[4]*basis[4][n] + in[5]*basis[5][n] + in[6]*basis[6][n] + in[7]*basis[7][n]
	}
}

// ForwardDCT2D transforms an 8x8 block of samples into 8x8 frequency
// coefficients. Element [0][0] of the result is the DC term (8 times the
// block average); the rest are AC terms of increasing frequency.
//
// The 2-D DCT is separable, so it is computed as a 1-D DCT over every row
// followed by a 1-D DCT over every column: 16 short transforms (about 1,000
// multiplications) instead of the 4,096 the direct double sum needs.
//
// Input samples are expected to be centered on zero, i.e. in [-128, 127].
func ForwardDCT2D(in [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	var rows, out block
	for r := 0; r < blockSize; r++ {
		forwardDCT1D(&in[r], &rows[r])
	}
	var col, res [blockSize]float64
	for c := 0; c < blockSize; c++ {
		for r := 0; r < blockSize; r++ {
			col[r] = rows[r][c]
		}
		forwardDCT1D(&col, &res)
		for r := 0; r < blockSize; r++ {
			out[r][c] = res[r]
		}
	}
	return out
}

// InverseDCT2D is the inverse of ForwardDCT2D: it turns 8x8 frequency
// coefficients back into zero-centered samples.
func InverseDCT2D(in [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	var rows, out block
	for r := 0; r < blockSize; r++ {
		inverseDCT1D(&in[r], &rows[r])
	}
	var col, res [blockSize]float64
	for c := 0; c < blockSize; c++ {
		for r := 0; r < blockSize; r++ {
			col[r] = rows[r][c]
		}
		inverseDCT1D(&col, &res)
		for r := 0; r < blockSize; r++ {
			out[r][c] = res[r]
		}
	}
	return out
}

// naiveForwardDCT2D evaluates the textbook 2-D DCT definition directly, with
// a cosine call per term. It exists as the reference the fast version is
// tested and benchmarked against.
func naiveForwardDCT2D(in [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	alpha := func(k int) float64 {
		if k == 0 {
			return 1 / math.Sqrt2
		}
		return 1
	}
	var out block
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			sum := 0.0
			for x := 0; x < blockSize; x++ {
				for y := 0; y < blockSize; y++ {
					sum += in[x][y] *
						math.Cos(float64(2*x+1)*float64(u)*math.Pi/16) *
						math.Cos(float64(2*y+1)*float64(v)*math.Pi/16)
				}
			}
			out[u][v] = 0.25 * alpha(u) * alpha(v) * sum
		}
	}
	return out
}
