package dct

import "math"

// Base quantization tables from Annex K of the JPEG standard (ITU-T T.81),
// tuned for quality 50. Each entry is the step size for one frequency:
// small steps at the top left (low frequencies, where the eye is sensitive)
// and large steps toward the bottom right (fine detail).

// standardLuminanceTable is the base table for the Y (brightness) channel.
var standardLuminanceTable = block{
	{16, 11, 10, 16, 24, 40, 51, 61},
	{12, 12, 14, 19, 26, 58, 60, 55},
	{14, 13, 16, 24, 40, 57, 69, 56},
	{14, 17, 22, 29, 51, 87, 80, 62},
	{18, 22, 37, 56, 68, 109, 103, 77},
	{24, 35, 55, 64, 81, 104, 113, 92},
	{49, 64, 78, 87, 103, 121, 120, 101},
	{72, 92, 95, 98, 112, 100, 103, 99},
}

// standardChrominanceTable is the base table for the Cb and Cr (color)
// channels. It is much coarser because the eye resolves color detail poorly.
var standardChrominanceTable = block{
	{17, 18, 24, 47, 99, 99, 99, 99},
	{18, 21, 26, 66, 99, 99, 99, 99},
	{24, 26, 56, 99, 99, 99, 99, 99},
	{47, 66, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
}

// dcStepScale shrinks the DC step relative to standard JPEG. The DC term
// carries the average brightness of a block, and errors in it show up as
// visible steps between neighbouring blocks. A finer step costs few bits
// because DC values are delta-coded.
const dcStepScale = 0.80

// qualityScaleFactor maps a quality setting (1-100) to the multiplier
// applied to the base tables, using the libjpeg convention: 50 leaves the
// tables unchanged, lower values scale the steps up (more loss, smaller
// file), higher values scale them down.
func qualityScaleFactor(quality int) float64 {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	if quality < 50 {
		return 50.0 / float64(quality)
	}
	return 2.0 - float64(quality)/50.0
}

// buildQuantizationTable scales a base table for the given quality. Steps
// are clamped to [1, 255]; a step of 1 means "round to the nearest integer".
//
// The decoder rebuilds the same table from the quality stored in the file
// header, so the table itself never has to be written out.
func buildQuantizationTable(base block, quality int) block {
	scale := qualityScaleFactor(quality)
	var table block
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			step := math.Round(base[u][v] * scale)
			if u == 0 && v == 0 {
				step = math.Round(step * dcStepScale)
			}
			table[u][v] = math.Min(255, math.Max(1, step))
		}
	}
	return table
}

// quantizeBlock divides every coefficient by its step and rounds to the
// nearest integer. This is the only lossy step in the codec: the rounding
// error cannot be undone. Most high-frequency coefficients become zero here,
// which is what makes the entropy coding that follows effective.
func quantizeBlock(coeffs *block, table *block) [blockSize][blockSize]int {
	var quantized [blockSize][blockSize]int
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			quantized[u][v] = int(math.Round(coeffs[u][v] / table[u][v]))
		}
	}
	return quantized
}

// dequantizeBlock multiplies each quantized value by its step, giving an
// approximation of the original coefficients.
func dequantizeBlock(quantized *[blockSize][blockSize]int, table *block) block {
	var coeffs block
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			coeffs[u][v] = float64(quantized[u][v]) * table[u][v]
		}
	}
	return coeffs
}
