package dct

import "math"

// standardLuminanceTable is the JPEG standard quantization table for the Y channel.
var standardLuminanceTable = [blockSize][blockSize]float64{
	{16, 11, 10, 16, 24, 40, 51, 61},
	{12, 12, 14, 19, 26, 58, 60, 55},
	{14, 13, 16, 24, 40, 57, 69, 56},
	{14, 17, 22, 29, 51, 87, 80, 62},
	{18, 22, 37, 56, 68, 109, 103, 77},
	{24, 35, 55, 64, 81, 104, 113, 92},
	{49, 64, 78, 87, 103, 121, 120, 101},
	{72, 92, 95, 98, 112, 100, 103, 99},
}

// standardChrominanceTable is the JPEG standard quantization table for Cb/Cr channels.
var standardChrominanceTable = [blockSize][blockSize]float64{
	{17, 18, 24, 47, 99, 99, 99, 99},
	{18, 21, 26, 66, 99, 99, 99, 99},
	{24, 26, 56, 99, 99, 99, 99, 99},
	{47, 66, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
	{99, 99, 99, 99, 99, 99, 99, 99},
}

// qualityScaleFactor maps a quality value (1–100) to a multiplier for the
// quantization table. Lower quality → larger divisors → more compression.
// Follows the JPEG reference formula.
func qualityScaleFactor(quality int) float64 {
	if quality <= 0 {
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

// buildQuantizationTable scales a base table by the quality factor.
// The DC coefficient gets a 25% tighter step (better low-frequency accuracy)
// while AC entries use the standard scale. Each entry is clamped to [1, 255].
func buildQuantizationTable(base [blockSize][blockSize]float64, quality int) [blockSize][blockSize]float64 {
	scale := qualityScaleFactor(quality)
	var table [blockSize][blockSize]float64
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			value := math.Round(base[u][v] * scale)
			if value < 1 {
				value = 1
			}
			if value > 255 {
				value = 255
			}
			// DC coefficient: reduce quantization step by 20% — finer DC
			// coding is the highest-leverage PSNR improvement available
			// without changing the rest of the pipeline.
			if u == 0 && v == 0 {
				value = math.Max(1, math.Round(value*0.80))
			}
			table[u][v] = value
		}
	}
	return table
}

// quantizeBlock divides each DCT coefficient by the table entry and rounds
// to the nearest integer — identical to standard JPEG for all AC coefficients.
// No deadzone is applied: every coefficient is preserved with full rounding
// accuracy. Combined with the 20%-tighter DC step and bilinear chroma
// upsampling on decode, this should yield higher PSNR than Go stdlib JPEG
// (which uses nearest-neighbor chroma upsampling) at the same file size.
func quantizeBlock(dctBlock [blockSize][blockSize]float64, table [blockSize][blockSize]float64) [blockSize][blockSize]int {
	var quantized [blockSize][blockSize]int
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			quantized[u][v] = int(math.Round(dctBlock[u][v] / table[u][v]))
		}
	}
	return quantized
}

// dequantizeBlock multiplies each quantized coefficient by the table entry,
// reconstructing approximate DCT coefficients for the inverse transform.
func dequantizeBlock(quantized [blockSize][blockSize]int, table [blockSize][blockSize]float64) [blockSize][blockSize]float64 {
	var dctBlock [blockSize][blockSize]float64
	for u := 0; u < blockSize; u++ {
		for v := 0; v < blockSize; v++ {
			dctBlock[u][v] = float64(quantized[u][v]) * table[u][v]
		}
	}
	return dctBlock
}
