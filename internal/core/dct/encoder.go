package dct

import (
	"encoding/binary"
	"fmt"
	"image"
	"sync"

	"imagecompressor-dct/internal/core/huffman"
	"imagecompressor-dct/internal/core/pixels"
	"imagecompressor-dct/internal/models"
)

// CompressionOptions holds the tunable encoder parameters.
type CompressionOptions struct {
	// Quality ranges from 1 (smallest file) to 100 (best fidelity).
	Quality int
	// ChromaSubsampling selects the resolution of the color channels.
	ChromaSubsampling ChromaSubsampling
}

// Encoder compresses images with a fixed set of options. It holds no
// per-image state, so one Encoder may be used from several goroutines.
type Encoder struct {
	options          CompressionOptions
	luminanceTable   block
	chrominanceTable block
}

// NewEncoder validates the options and precomputes the quantization tables.
func NewEncoder(options CompressionOptions) (*Encoder, error) {
	if options.Quality < 1 || options.Quality > 100 {
		return nil, fmt.Errorf("quality must be between 1 and 100, got %d", options.Quality)
	}
	if !options.ChromaSubsampling.valid() {
		return nil, fmt.Errorf("unknown chroma subsampling mode %d", options.ChromaSubsampling)
	}
	return &Encoder{
		options:          options,
		luminanceTable:   buildQuantizationTable(standardLuminanceTable, options.Quality),
		chrominanceTable: buildQuantizationTable(standardChrominanceTable, options.Quality),
	}, nil
}

// Encode compresses img into the .dct format.
//
// Pipeline:
//
//  1. RGB to YCbCr, separating brightness (Y) from color (Cb, Cr)
//  2. optional downscaling of the two color planes
//  3. per plane, per 8x8 block: DCT, quantize, reorder in zigzag
//  4. per plane: delta-code the DC terms and cut trailing zeros
//  5. per plane: Huffman-code the resulting integer stream
//  6. write the header followed by the three streams
//
// Steps 1 and 3 run on a pool of worker goroutines.
func (e *Encoder) Encode(img image.Image) (*models.CompressionResult, error) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 1 || height < 1 {
		return nil, fmt.Errorf("image is empty (%dx%d)", width, height)
	}
	if width > maxDimension || height > maxDimension {
		return nil, fmt.Errorf("image too large (%dx%d, limit %d per side)", width, height, maxDimension)
	}

	yPlane, cbPlane, crPlane := toYCbCr(pixels.ToRGBA(img))

	switch e.options.ChromaSubsampling {
	case Subsampling420:
		cbPlane, crPlane = downsample(cbPlane, 2, 2), downsample(crPlane, 2, 2)
	case Subsampling422:
		cbPlane, crPlane = downsample(cbPlane, 2, 1), downsample(crPlane, 2, 1)
	}

	// Each plane already uses every core for its blocks, so the planes are
	// transformed one after the other. Entropy coding is sequential within
	// a plane, so there the three planes run side by side instead.
	channels := [3]struct {
		plane *plane
		table *block
	}{
		{yPlane, &e.luminanceTable},
		{cbPlane, &e.chrominanceTable},
		{crPlane, &e.chrominanceTable},
	}

	var (
		streams [3][]byte
		errs    [3]error
		wg      sync.WaitGroup
	)
	for i, ch := range channels {
		symbols := compressPlane(ch.plane, ch.table)
		wg.Add(1)
		go func(i int, symbols []int) {
			defer wg.Done()
			streams[i], errs[i] = huffman.NewEncoder(symbols).Encode(symbols)
		}(i, symbols)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("entropy coding failed: %w", err)
		}
	}

	packed := pack(width, height, e.options, streams)

	// The ratio is measured against uncompressed 24-bit RGB.
	originalSize := width * height * 3
	return &models.CompressionResult{
		Data:             packed,
		OriginalSize:     originalSize,
		CompressedSize:   len(packed),
		CompressionRatio: float64(originalSize) / float64(len(packed)),
		Width:            width,
		Height:           height,
		Quality:          e.options.Quality,
	}, nil
}

// toYCbCr splits an RGBA image into Y, Cb and Cr planes using the JFIF
// (BT.601 full-range) conversion. Y is shifted down by 128 so that all three
// planes are centered on zero, which is what the DCT expects.
func toYCbCr(img *image.RGBA) (y, cb, cr *plane) {
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	y, cb, cr = newPlane(width, height), newPlane(width, height), newPlane(width, height)

	forEachRow(height, func(r int) {
		src := img.Pix[r*img.Stride:]
		yRow, cbRow, crRow := y.row(r), cb.row(r), cr.row(r)
		for c := 0; c < width; c++ {
			red := float64(src[4*c])
			green := float64(src[4*c+1])
			blue := float64(src[4*c+2])

			yRow[c] = 0.299*red + 0.587*green + 0.114*blue - 128
			cbRow[c] = -0.168736*red - 0.331264*green + 0.5*blue
			crRow[c] = 0.5*red - 0.418688*green - 0.081312*blue
		}
	})
	return y, cb, cr
}

// downsample shrinks a plane by the given integer factors, replacing each
// factorX x factorY cell with its average. Cells that hang over the right or
// bottom edge are averaged over the samples that exist.
func downsample(src *plane, factorX, factorY int) *plane {
	dst := newPlane(ceilDiv(src.width, factorX), ceilDiv(src.height, factorY))

	forEachRow(dst.height, func(r int) {
		out := dst.row(r)
		for c := range out {
			sum, count := 0.0, 0
			for dy := 0; dy < factorY; dy++ {
				sr := r*factorY + dy
				if sr >= src.height {
					break
				}
				row := src.row(sr)
				for dx := 0; dx < factorX; dx++ {
					if sc := c*factorX + dx; sc < src.width {
						sum += row[sc]
						count++
					}
				}
			}
			out[c] = sum / float64(count)
		}
	})
	return dst
}

// compressPlane turns one plane into the integer stream that is handed to
// the entropy coder.
//
// The transform of each block is independent of every other block, so that
// part is spread across the worker pool, one row of blocks per task. Writing
// the stream is done afterwards in a single pass, because every DC value is
// stored as the difference from the previous block's DC value.
func compressPlane(p *plane, table *block) []int {
	blocksWide := ceilDiv(p.width, blockSize)
	blocksHigh := ceilDiv(p.height, blockSize)

	// quantized holds every block's 64 coefficients in zigzag order.
	quantized := make([]int16, blocksWide*blocksHigh*64)

	forEachRow(blocksHigh, func(br int) {
		var samples block
		for bc := 0; bc < blocksWide; bc++ {
			loadBlock(p, br*blockSize, bc*blockSize, &samples)
			coeffs := ForwardDCT2D(samples)
			q := quantizeBlock(&coeffs, table)

			out := quantized[(br*blocksWide+bc)*64:][:64]
			for i, pos := range zigzag {
				out[i] = int16(q[pos/blockSize][pos%blockSize])
			}
		}
	})

	// Stream layout per block: DC delta, AC coefficients up to the last
	// non-zero one, then the end-of-block marker.
	//
	// Neighbouring blocks usually have similar brightness, so the DC deltas
	// are small numbers that repeat often. After quantization most blocks
	// are zero beyond the first few coefficients, so the marker typically
	// replaces dozens of zeros.
	symbols := make([]int, 0, len(quantized)/6)
	previousDC := 0
	for b := 0; b < len(quantized); b += 64 {
		coeffs := quantized[b : b+64]

		dc := int(coeffs[0])
		symbols = append(symbols, dc-previousDC)
		previousDC = dc

		last := 63
		for last > 0 && coeffs[last] == 0 {
			last--
		}
		for i := 1; i <= last; i++ {
			symbols = append(symbols, int(coeffs[i]))
		}
		symbols = append(symbols, eobMarker)
	}
	return symbols
}

// loadBlock copies the 8x8 block whose top-left corner is (top, left) into
// dst. Where the block extends past the plane, the nearest edge sample is
// repeated. Padding with a repeat of the edge, rather than with zeros,
// avoids creating an artificial sharp edge that would cost bits to encode.
func loadBlock(p *plane, top, left int, dst *block) {
	for r := 0; r < blockSize; r++ {
		sr := top + r
		if sr >= p.height {
			sr = p.height - 1
		}
		row := p.row(sr)
		for c := 0; c < blockSize; c++ {
			sc := left + c
			if sc >= p.width {
				sc = p.width - 1
			}
			dst[r][c] = row[sc]
		}
	}
}

// pack writes the file header followed by the three entropy-coded streams.
func pack(width, height int, options CompressionOptions, streams [3][]byte) []byte {
	size := headerSize
	for _, s := range streams {
		size += len(s)
	}

	buf := make([]byte, 0, size)
	buf = append(buf, magic[:]...)
	buf = append(buf, formatVersion)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(width))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(height))
	buf = append(buf, byte(options.Quality), byte(options.ChromaSubsampling))
	for _, s := range streams {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(s)))
	}
	for _, s := range streams {
		buf = append(buf, s...)
	}
	return buf
}
