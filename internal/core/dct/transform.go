// Package dct implements Discrete Cosine Transform-based image compression
// following the JPEG baseline specification. It provides encoding and decoding
// with tunable quality parameters and chroma subsampling options.
package dct

import "math"

const blockSize = 8

// precomputedCosines holds cos((2x+1)*u*π/16) for all x,u in [0,8).
var precomputedCosines [blockSize][blockSize]float64

func init() {
	for u := 0; u < blockSize; u++ {
		for x := 0; x < blockSize; x++ {
			precomputedCosines[u][x] = math.Cos((2*float64(x)+1) * float64(u) * math.Pi / 16.0)
		}
	}
}

// alphaFactor returns the DCT normalisation coefficient: 1/√2 for k=0, 1 otherwise.
func alphaFactor(k int) float64 {
	if k == 0 {
		return 1.0 / math.Sqrt2
	}
	return 1.0
}

// forwardDCT1D computes the 1-D DCT-II of an 8-element array.
// Output[k] = 0.5 · α(k) · Σ_n input[n] · cos((2n+1)kπ/16)
// When applied separably (rows then columns), two passes reproduce ForwardDCT2D exactly.
func forwardDCT1D(x [blockSize]float64) [blockSize]float64 {
	var out [blockSize]float64
	for k := 0; k < blockSize; k++ {
		sum := 0.0
		for n := 0; n < blockSize; n++ {
			sum += x[n] * precomputedCosines[k][n]
		}
		out[k] = 0.5 * alphaFactor(k) * sum
	}
	return out
}

// inverseDCT1D computes the 1-D IDCT-II of an 8-element frequency array.
// Output[n] = 0.5 · Σ_k α(k) · X[k] · cos((2n+1)kπ/16)
func inverseDCT1D(X [blockSize]float64) [blockSize]float64 {
	var out [blockSize]float64
	for n := 0; n < blockSize; n++ {
		sum := 0.0
		for k := 0; k < blockSize; k++ {
			sum += alphaFactor(k) * X[k] * precomputedCosines[k][n]
		}
		out[n] = 0.5 * sum
	}
	return out
}

// ForwardDCT2D applies the 2D DCT to an 8×8 block using two separable 1D passes
// (row-wise then column-wise). Complexity O(n³) vs O(n⁴) for the naive form.
// Input values should be level-shifted to [-128, 127].
func ForwardDCT2D(block [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	var tmp [blockSize][blockSize]float64
	// Pass 1: 1D DCT along each row → tmp[x][v]
	for x := 0; x < blockSize; x++ {
		tmp[x] = forwardDCT1D(block[x])
	}
	// Pass 2: 1D DCT along each column of tmp → result[u][v]
	var result [blockSize][blockSize]float64
	for v := 0; v < blockSize; v++ {
		var col [blockSize]float64
		for x := 0; x < blockSize; x++ {
			col[x] = tmp[x][v]
		}
		dct := forwardDCT1D(col)
		for u := 0; u < blockSize; u++ {
			result[u][v] = dct[u]
		}
	}
	return result
}

// InverseDCT2D applies the 2D IDCT to an 8×8 frequency block using two separable
// 1D passes. Returns pixel values in [-128, 127] before level-shifting back to [0, 255].
func InverseDCT2D(dctBlock [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	// Pass 1: 1D IDCT along each row (frequency → intermediate spatial in y)
	var tmp [blockSize][blockSize]float64
	for u := 0; u < blockSize; u++ {
		tmp[u] = inverseDCT1D(dctBlock[u])
	}
	// Pass 2: 1D IDCT along each column (intermediate → spatial in x)
	var result [blockSize][blockSize]float64
	for y := 0; y < blockSize; y++ {
		var col [blockSize]float64
		for u := 0; u < blockSize; u++ {
			col[u] = tmp[u][y]
		}
		idct := inverseDCT1D(col)
		for x := 0; x < blockSize; x++ {
			result[x][y] = idct[x]
		}
	}
	return result
}
