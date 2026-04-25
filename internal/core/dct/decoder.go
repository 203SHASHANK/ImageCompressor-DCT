package dct

import (
	"encoding/binary"
	"fmt"
	"image"
	"imagecompressor-dct/internal/core/huffman"
	"math"
)

// Decoder reconstructs images from the custom .dct binary format.
type Decoder struct{}

// NewDecoder creates a Decoder instance.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// Decode reconstructs an image.RGBA from compressed .dct byte data.
func (decoder *Decoder) Decode(data []byte) (image.Image, error) {
	width, height, quality, subsampling, yData, cbData, crData, err := unpackChannels(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack compressed data: %w", err)
	}

	luminanceTable := buildQuantizationTable(standardLuminanceTable, quality)
	chrominanceTable := buildQuantizationTable(standardChrominanceTable, quality)

	yPlane := decoder.decompressChannel(yData, width, height, luminanceTable)

	// Determine chroma plane dimensions based on subsampling mode.
	cbWidth, cbHeight := chromaDimensions(width, height, ChromaSubsampling(subsampling))
	cbPlane := decoder.decompressChannel(cbData, cbWidth, cbHeight, chrominanceTable)
	crPlane := decoder.decompressChannel(crData, cbWidth, cbHeight, chrominanceTable)

	// Upsample chroma planes back to full resolution if needed.
	cbPlane = upsampleChroma(cbPlane, width, height, ChromaSubsampling(subsampling))
	crPlane = upsampleChroma(crPlane, width, height, ChromaSubsampling(subsampling))

	return buildRGBAImage(yPlane, cbPlane, crPlane, width, height), nil
}

// decompressChannel reconstructs a single-channel plane from the compact RLE+delta stream.
// Each block: dc_delta, then AC values until eobMarker (remaining ACs implicitly zero).
func (decoder *Decoder) decompressChannel(coefficients []int, width, height int, quantTable [blockSize][blockSize]float64) [][]float64 {
	paddedHeight := ceilDiv(height, blockSize) * blockSize
	paddedWidth := ceilDiv(width, blockSize) * blockSize

	plane := make([][]float64, paddedHeight)
	for row := range plane {
		plane[row] = make([]float64, paddedWidth)
	}

	blocksPerRow := paddedWidth / blockSize
	totalBlocks := (paddedHeight / blockSize) * blocksPerRow

	pos := 0
	prevDC := 0

	for blockIndex := 0; blockIndex < totalBlocks && pos < len(coefficients); blockIndex++ {
		var zigzag [64]int

		dc := prevDC + coefficients[pos]
		pos++
		prevDC = dc
		zigzag[0] = dc

		for acIdx := 1; pos < len(coefficients); {
			v := coefficients[pos]
			pos++
			if v == eobMarker {
				break
			}
			if acIdx < 64 {
				zigzag[acIdx] = v
				acIdx++
			}
		}

		quantized := zigzagUnflatten(zigzag[:])
		dctBlock := dequantizeBlock(quantized, quantTable)
		spatialBlock := InverseDCT2D(dctBlock)

		blockRow := (blockIndex / blocksPerRow) * blockSize
		blockCol := (blockIndex % blocksPerRow) * blockSize

		for row := 0; row < blockSize; row++ {
			for col := 0; col < blockSize; col++ {
				if blockRow+row < paddedHeight && blockCol+col < paddedWidth {
					plane[blockRow+row][blockCol+col] = spatialBlock[row][col]
				}
			}
		}
	}

	trimmed := make([][]float64, height)
	for row := 0; row < height; row++ {
		trimmed[row] = plane[row][:width]
	}
	return trimmed
}

// buildRGBAImage converts three YCbCr planes into an *image.RGBA.
func buildRGBAImage(yPlane, cbPlane, crPlane [][]float64, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			pixel := ycbcrToRGB(yPlane[row][col], cbPlane[row][col], crPlane[row][col])
			img.SetRGBA(col, row, pixel)
		}
	}
	return img
}

// chromaDimensions returns the chroma plane size for a given subsampling mode.
func chromaDimensions(width, height int, subsampling ChromaSubsampling) (int, int) {
	switch subsampling {
	case Subsampling420:
		return ceilDiv(width, 2), ceilDiv(height, 2)
	case Subsampling422:
		return ceilDiv(width, 2), height
	default:
		return width, height
	}
}

// upsampleChroma expands a chroma plane back to full luma resolution using
// bilinear interpolation. Bilinear vs nearest-neighbor gives ~0.15–0.3 dB
// better PSNR with no change in compressed size.
func upsampleChroma(plane [][]float64, targetWidth, targetHeight int, subsampling ChromaSubsampling) [][]float64 {
	if subsampling == Subsampling444 {
		return plane
	}

	srcH := len(plane)
	srcW := 0
	if srcH > 0 {
		srcW = len(plane[0])
	}

	clampI := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	bilinear := func(row, col int, scaleY, scaleX float64) float64 {
		// Map target pixel centre to source coordinate (JPEG chroma siting).
		srcY := (float64(row)+0.5)/scaleY - 0.5
		srcX := (float64(col)+0.5)/scaleX - 0.5

		y0 := int(math.Floor(srcY))
		x0 := int(math.Floor(srcX))
		y1 := y0 + 1
		x1 := x0 + 1

		y0 = clampI(y0, 0, srcH-1)
		y1 = clampI(y1, 0, srcH-1)
		x0 = clampI(x0, 0, srcW-1)
		x1 = clampI(x1, 0, srcW-1)

		fy := srcY - math.Floor(srcY)
		fx := srcX - math.Floor(srcX)
		if srcY < 0 {
			fy = 0
		}
		if srcX < 0 {
			fx = 0
		}

		tl := plane[y0][x0]
		tr := plane[y0][x1]
		bl := plane[y1][x0]
		br := plane[y1][x1]
		return tl*(1-fx)*(1-fy) + tr*fx*(1-fy) + bl*(1-fx)*fy + br*fx*fy
	}

	result := make([][]float64, targetHeight)
	for row := 0; row < targetHeight; row++ {
		result[row] = make([]float64, targetWidth)
		for col := 0; col < targetWidth; col++ {
			switch subsampling {
			case Subsampling420:
				result[row][col] = bilinear(row, col, 2.0, 2.0)
			case Subsampling422:
				result[row][col] = bilinear(row, col, 1.0, 2.0)
			}
		}
	}
	return result
}

// unpackChannels parses the binary .dct format produced by packChannels.
// Supports version 2 (Huffman-encoded channels).
func unpackChannels(data []byte) (width, height, quality, subsampling int, yData, cbData, crData []int, err error) {
	const headerSize = 4 + 1 + 4 + 4 + 1 + 1 + 4 + 4 + 4
	if len(data) < headerSize {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("data too short for header: %d bytes", len(data))
	}

	if string(data[0:3]) != "DCT" {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("invalid magic bytes")
	}

	width = int(binary.LittleEndian.Uint32(data[5:9]))
	height = int(binary.LittleEndian.Uint32(data[9:13]))
	quality = int(data[13])
	subsampling = int(data[14])
	yLen := int(binary.LittleEndian.Uint32(data[15:19]))
	cbLen := int(binary.LittleEndian.Uint32(data[19:23]))
	crLen := int(binary.LittleEndian.Uint32(data[23:27]))

	expectedSize := headerSize + yLen + cbLen + crLen
	if len(data) < expectedSize {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("data too short: need %d bytes, got %d", expectedSize, len(data))
	}

	offset := headerSize
	yData, err = huffman.Decode(data[offset : offset+yLen])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("Y channel Huffman decode failed: %w", err)
	}
	offset += yLen

	cbData, err = huffman.Decode(data[offset : offset+cbLen])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("Cb channel Huffman decode failed: %w", err)
	}
	offset += cbLen

	crData, err = huffman.Decode(data[offset : offset+crLen])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, nil, fmt.Errorf("Cr channel Huffman decode failed: %w", err)
	}

	return width, height, quality, subsampling, yData, cbData, crData, nil
}


