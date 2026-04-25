package dct

import (
	"fmt"
	"image"
	"image/color"
	"imagecompressor-dct/internal/core/huffman"
	"imagecompressor-dct/internal/models"
	"runtime"
	"sync"
)

// eobMarker signals end-of-block in the RLE coefficient stream. All trailing
// AC coefficients after this point in the current block are zero. The value
// -32768 (math.MinInt16) cannot appear as a normal quantized DCT coefficient.
const eobMarker = -32768

// ChromaSubsampling defines the color resolution reduction mode.
type ChromaSubsampling int

const (
	// Subsampling444 keeps full chroma resolution (no subsampling).
	Subsampling444 ChromaSubsampling = iota
	// Subsampling422 halves horizontal chroma resolution.
	Subsampling422
	// Subsampling420 halves both horizontal and vertical chroma resolution.
	Subsampling420
)

// CompressionOptions defines tunable parameters for DCT encoding.
type CompressionOptions struct {
	// Quality ranges from 1 (maximum compression) to 100 (minimum compression).
	Quality int
	// ChromaSubsampling controls color channel resolution reduction.
	ChromaSubsampling ChromaSubsampling
}

// Encoder performs DCT-based image compression.
type Encoder struct {
	options               CompressionOptions
	luminanceQuantTable   [blockSize][blockSize]float64
	chrominanceQuantTable [blockSize][blockSize]float64
}

// NewEncoder creates a configured Encoder. Quality must be in [1, 100].
func NewEncoder(options CompressionOptions) (*Encoder, error) {
	if options.Quality < 1 || options.Quality > 100 {
		return nil, fmt.Errorf("quality must be between 1 and 100, got %d", options.Quality)
	}
	return &Encoder{
		options:               options,
		luminanceQuantTable:   buildQuantizationTable(standardLuminanceTable, options.Quality),
		chrominanceQuantTable: buildQuantizationTable(standardChrominanceTable, options.Quality),
	}, nil
}

// Encode compresses an image using the DCT pipeline and returns a CompressionResult.
// The pipeline: RGB→YCbCr → block split → DCT → quantize → zigzag → pack.
func (encoder *Encoder) Encode(img image.Image) (*models.CompressionResult, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Convert to YCbCr planes.
	yPlane, cbPlane, crPlane := extractYCbCrPlanes(img)

	// Subsample chroma channels if requested.
	cbPlane, crPlane = encoder.subsampleChroma(cbPlane, crPlane, width, height)

	// Compress each channel independently.
	yData := encoder.compressChannel(yPlane, encoder.luminanceQuantTable)
	cbData := encoder.compressChannel(cbPlane, encoder.chrominanceQuantTable)
	crData := encoder.compressChannel(crPlane, encoder.chrominanceQuantTable)

	// Pack into a simple binary format: [header][Y blocks][Cb blocks][Cr blocks].
	packed := packChannels(width, height, encoder.options.Quality, int(encoder.options.ChromaSubsampling), yData, cbData, crData)

	originalSize := width * height * 3 // RGB bytes
	return &models.CompressionResult{
		Data:             packed,
		OriginalSize:     originalSize,
		CompressedSize:   len(packed),
		CompressionRatio: float64(originalSize) / float64(len(packed)),
		Width:            width,
		Height:           height,
		Quality:          encoder.options.Quality,
	}, nil
}

// compressChannel encodes a single-channel plane to a compact integer stream.
//
// Each 8×8 block is emitted as: [dc_delta] [ac_1 … ac_k] [eobMarker], where k
// is the index of the last non-zero AC coefficient. Trailing zeros are dropped
// (replaced by eobMarker), and the DC coefficient is delta-encoded relative to
// the previous block. These two techniques together typically cut the symbol
// count 4–8× compared with emitting all 64 raw coefficients per block.
//
// DCT + quantize + zigzag are run in parallel across a worker pool; DC delta
// encoding is serial (each block depends on its predecessor's DC value).
func (encoder *Encoder) compressChannel(plane [][]float64, quantTable [blockSize][blockSize]float64) []int {
	height := len(plane)
	if height == 0 {
		return nil
	}
	width := len(plane[0])

	paddedHeight := ceilDiv(height, blockSize) * blockSize
	paddedWidth := ceilDiv(width, blockSize) * blockSize
	numBlockCols := paddedWidth / blockSize
	totalBlocks := (paddedHeight / blockSize) * numBlockCols

	type blockWork struct{ idx, row, col int }
	type blockResult struct{ zigzag [64]int }

	processed := make([]blockResult, totalBlocks)

	numWorkers := runtime.NumCPU()
	if numWorkers > totalBlocks {
		numWorkers = totalBlocks
	}

	workChan := make(chan blockWork, totalBlocks)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range workChan {
				block := extractBlock(plane, work.row, work.col, height, width)
				dctBlock := ForwardDCT2D(block)
				quantized := quantizeBlock(dctBlock, quantTable)
				processed[work.idx].zigzag = zigzagFlattenFixed(quantized)
			}
		}()
	}

	for idx := 0; idx < totalBlocks; idx++ {
		workChan <- blockWork{
			idx: idx,
			row: (idx / numBlockCols) * blockSize,
			col: (idx % numBlockCols) * blockSize,
		}
	}
	close(workChan)
	wg.Wait()

	// Serial DC delta-encoding and packing (order must be preserved).
	coefficients := make([]int, 0, totalBlocks*10)
	prevDC := 0

	for i := 0; i < totalBlocks; i++ {
		zz := processed[i].zigzag
		dcDelta := zz[0] - prevDC
		prevDC = zz[0]

		lastNonZero := 0
		for k := 63; k >= 1; k-- {
			if zz[k] != 0 {
				lastNonZero = k
				break
			}
		}

		coefficients = append(coefficients, dcDelta)
		for k := 1; k <= lastNonZero; k++ {
			coefficients = append(coefficients, zz[k])
		}
		coefficients = append(coefficients, eobMarker)
	}

	return coefficients
}

// subsampleChroma reduces chroma plane resolution based on the selected mode.
func (encoder *Encoder) subsampleChroma(cbPlane, crPlane [][]float64, width, height int) ([][]float64, [][]float64) {
	switch encoder.options.ChromaSubsampling {
	case Subsampling420:
		return downsample2D(cbPlane, width, height), downsample2D(crPlane, width, height)
	case Subsampling422:
		return downsampleHorizontal(cbPlane, width, height), downsampleHorizontal(crPlane, width, height)
	default:
		return cbPlane, crPlane
	}
}

// extractYCbCrPlanes converts an image to three separate float64 planes (Y, Cb, Cr).
// Pixel values are level-shifted by -128 to center around zero for DCT.
func extractYCbCrPlanes(img image.Image) ([][]float64, [][]float64, [][]float64) {
	bounds := img.Bounds()
	height := bounds.Dy()
	width := bounds.Dx()

	yPlane := make([][]float64, height)
	cbPlane := make([][]float64, height)
	crPlane := make([][]float64, height)

	for row := 0; row < height; row++ {
		yPlane[row] = make([]float64, width)
		cbPlane[row] = make([]float64, width)
		crPlane[row] = make([]float64, width)

		for col := 0; col < width; col++ {
			r, g, b, _ := img.At(bounds.Min.X+col, bounds.Min.Y+row).RGBA()
			rf := float64(r >> 8)
			gf := float64(g >> 8)
			bf := float64(b >> 8)

			yPlane[row][col] = 0.299*rf + 0.587*gf + 0.114*bf - 128.0
			cbPlane[row][col] = -0.168736*rf - 0.331264*gf + 0.5*bf
			crPlane[row][col] = 0.5*rf - 0.418688*gf - 0.081312*bf
		}
	}

	return yPlane, cbPlane, crPlane
}

// extractBlock copies an 8×8 region from a plane, padding with edge values if needed.
func extractBlock(plane [][]float64, startRow, startCol, planeHeight, planeWidth int) [blockSize][blockSize]float64 {
	var block [blockSize][blockSize]float64
	for row := 0; row < blockSize; row++ {
		for col := 0; col < blockSize; col++ {
			srcRow := startRow + row
			srcCol := startCol + col
			if srcRow >= planeHeight {
				srcRow = planeHeight - 1
			}
			if srcCol >= planeWidth {
				srcCol = planeWidth - 1
			}
			block[row][col] = plane[srcRow][srcCol]
		}
	}
	return block
}

// zigzagOrder defines the 64-element zigzag traversal of an 8×8 block.
var zigzagOrder = [64][2]int{
	{0, 0}, {0, 1}, {1, 0}, {2, 0}, {1, 1}, {0, 2}, {0, 3}, {1, 2},
	{2, 1}, {3, 0}, {4, 0}, {3, 1}, {2, 2}, {1, 3}, {0, 4}, {0, 5},
	{1, 4}, {2, 3}, {3, 2}, {4, 1}, {5, 0}, {6, 0}, {5, 1}, {4, 2},
	{3, 3}, {2, 4}, {1, 5}, {0, 6}, {0, 7}, {1, 6}, {2, 5}, {3, 4},
	{4, 3}, {5, 2}, {6, 1}, {7, 0}, {7, 1}, {6, 2}, {5, 3}, {4, 4},
	{3, 5}, {2, 6}, {1, 7}, {2, 7}, {3, 6}, {4, 5}, {5, 4}, {6, 3},
	{7, 2}, {7, 3}, {6, 4}, {5, 5}, {4, 6}, {3, 7}, {4, 7}, {5, 6},
	{6, 5}, {7, 4}, {7, 5}, {6, 6}, {5, 7}, {6, 7}, {7, 6}, {7, 7},
}

// zigzagFlatten reorders an 8×8 quantized block into a 64-element slice
// using the zigzag pattern, grouping low-frequency coefficients first.
func zigzagFlatten(block [blockSize][blockSize]int) []int {
	coefficients := make([]int, 64)
	for index, position := range zigzagOrder {
		coefficients[index] = block[position[0]][position[1]]
	}
	return coefficients
}

// zigzagFlattenFixed is like zigzagFlatten but returns a fixed-size [64]int,
// avoiding a heap allocation in the parallel hot path.
func zigzagFlattenFixed(block [blockSize][blockSize]int) [64]int {
	var coefficients [64]int
	for index, position := range zigzagOrder {
		coefficients[index] = block[position[0]][position[1]]
	}
	return coefficients
}

// zigzagUnflatten reconstructs an 8×8 block from a 64-element zigzag slice.
func zigzagUnflatten(coefficients []int) [blockSize][blockSize]int {
	var block [blockSize][blockSize]int
	for index, position := range zigzagOrder {
		block[position[0]][position[1]] = coefficients[index]
	}
	return block
}

// downsample2D halves both dimensions of a plane by averaging 2×2 regions.
func downsample2D(plane [][]float64, width, height int) [][]float64 {
	newHeight := ceilDiv(height, 2)
	newWidth := ceilDiv(width, 2)
	result := make([][]float64, newHeight)
	for row := 0; row < newHeight; row++ {
		result[row] = make([]float64, newWidth)
		for col := 0; col < newWidth; col++ {
			result[row][col] = averageRegion(plane, row*2, col*2, height, width)
		}
	}
	return result
}

// downsampleHorizontal halves only the horizontal dimension of a plane.
func downsampleHorizontal(plane [][]float64, width, height int) [][]float64 {
	newWidth := ceilDiv(width, 2)
	result := make([][]float64, height)
	for row := 0; row < height; row++ {
		result[row] = make([]float64, newWidth)
		for col := 0; col < newWidth; col++ {
			srcCol := col * 2
			val := plane[row][srcCol]
			if srcCol+1 < width {
				val = (val + plane[row][srcCol+1]) / 2.0
			}
			result[row][col] = val
		}
	}
	return result
}

// averageRegion computes the average of up to a 2×2 region starting at (row, col).
func averageRegion(plane [][]float64, row, col, height, width int) float64 {
	sum := 0.0
	count := 0.0
	for dr := 0; dr < 2; dr++ {
		for dc := 0; dc < 2; dc++ {
			r := row + dr
			c := col + dc
			if r < height && c < width {
				sum += plane[r][c]
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

// ceilDiv returns ⌈a/b⌉ for positive integers.
func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

// packChannels serializes compressed channel data into a byte slice using Huffman coding.
// Format: [magic(4)][version(1)][width(4)][height(4)][quality(1)][subsampling(1)]
//
//	[yHuffLen(4)][cbHuffLen(4)][crHuffLen(4)][yHuffBytes...][cbHuffBytes...][crHuffBytes...]
//
// Each channel's coefficients are Huffman-encoded independently.
func packChannels(width, height, quality, subsampling int, yData, cbData, crData []int) []byte {
	yEncoded := huffmanEncodeChannel(yData)
	cbEncoded := huffmanEncodeChannel(cbData)
	crEncoded := huffmanEncodeChannel(crData)

	const headerSize = 4 + 1 + 4 + 4 + 1 + 1 + 4 + 4 + 4
	buf := make([]byte, 0, headerSize+len(yEncoded)+len(cbEncoded)+len(crEncoded))

	// Magic: "DCT\x02" (version 2 = Huffman-encoded channels)
	buf = append(buf, 'D', 'C', 'T', 0x02)
	buf = append(buf, 2) // version
	buf = appendUint32LE(buf, uint32(width))
	buf = appendUint32LE(buf, uint32(height))
	buf = append(buf, byte(quality))
	buf = append(buf, byte(subsampling))
	buf = appendUint32LE(buf, uint32(len(yEncoded)))
	buf = appendUint32LE(buf, uint32(len(cbEncoded)))
	buf = appendUint32LE(buf, uint32(len(crEncoded)))
	buf = append(buf, yEncoded...)
	buf = append(buf, cbEncoded...)
	buf = append(buf, crEncoded...)

	return buf
}

// huffmanEncodeChannel Huffman-encodes a coefficient slice.
// Returns an empty slice if the input is empty.
func huffmanEncodeChannel(coefficients []int) []byte {
	if len(coefficients) == 0 {
		return []byte{}
	}
	encoder := huffman.NewEncoder(coefficients)
	encoded, err := encoder.Encode(coefficients)
	if err != nil {
		// Fallback: store raw int16 if Huffman fails (should not happen in practice).
		fallback := make([]byte, len(coefficients)*2)
		for i, coeff := range coefficients {
			fallback[i*2] = byte(coeff)
			fallback[i*2+1] = byte(coeff >> 8)
		}
		return fallback
	}
	return encoded
}

// appendUint32LE appends a uint32 in little-endian order to a byte slice.
func appendUint32LE(buf []byte, v uint32) []byte {
	return append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

// ycbcrToRGB converts YCbCr float values back to an RGBA color.
// Input Y is level-shifted (centered at 0); Cb and Cr are also centered.
func ycbcrToRGB(yVal, cbVal, crVal float64) color.RGBA {
	yVal += 128.0
	red := yVal + 1.402*crVal
	green := yVal - 0.344136*cbVal - 0.714136*crVal
	blue := yVal + 1.772*cbVal

	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}

	return color.RGBA{R: clamp(red), G: clamp(green), B: clamp(blue), A: 255}
}
