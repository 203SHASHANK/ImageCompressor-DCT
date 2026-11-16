package dct

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"imagecompressor-dct/internal/core/huffman"
)

// Decoder reconstructs images from .dct data. It is stateless and safe for
// concurrent use.
type Decoder struct{}

// NewDecoder creates a Decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// header is the parsed fixed-size part of a .dct file.
type header struct {
	width, height int
	quality       int
	subsampling   ChromaSubsampling
	streams       [3][]byte // Y, Cb, Cr, still entropy-coded
}

// Decode reconstructs an image from .dct data.
//
// The input is validated as untrusted: a damaged or hostile file produces an
// error, never a panic or an oversized allocation.
func (d *Decoder) Decode(data []byte) (image.Image, error) {
	h, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	luminanceTable := buildQuantizationTable(standardLuminanceTable, h.quality)
	chrominanceTable := buildQuantizationTable(standardChrominanceTable, h.quality)
	chromaWidth, chromaHeight := chromaDimensions(h.width, h.height, h.subsampling)

	channels := [3]struct {
		name          string
		width, height int
		table         *block
	}{
		{"Y", h.width, h.height, &luminanceTable},
		{"Cb", chromaWidth, chromaHeight, &chrominanceTable},
		{"Cr", chromaWidth, chromaHeight, &chrominanceTable},
	}

	// Entropy decoding is sequential within a channel, so the three channels
	// are decoded side by side.
	var (
		symbols [3][]int
		errs    [3]error
		wg      sync.WaitGroup
	)
	for i, ch := range channels {
		wg.Add(1)
		go func(i, blocks int) {
			defer wg.Done()
			// A block contributes at most 64 coefficients plus a marker.
			symbols[i], errs[i] = huffman.DecodeLimit(h.streams[i], blocks*65)
		}(i, ceilDiv(ch.width, blockSize)*ceilDiv(ch.height, blockSize))
	}
	wg.Wait()

	var planes [3]*plane
	for i, ch := range channels {
		if errs[i] != nil {
			return nil, fmt.Errorf("%s channel: %w", ch.name, errs[i])
		}
		planes[i], err = decompressPlane(symbols[i], ch.width, ch.height, ch.table)
		if err != nil {
			return nil, fmt.Errorf("%s channel: %w", ch.name, err)
		}
	}

	if h.subsampling != Subsampling444 {
		planes[1] = upsample(planes[1], h.width, h.height)
		planes[2] = upsample(planes[2], h.width, h.height)
	}
	return toRGBA(planes[0], planes[1], planes[2]), nil
}

// parseHeader validates the fixed header and slices out the three streams.
func parseHeader(data []byte) (*header, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("data too short for header: %d bytes", len(data))
	}
	if !bytes.Equal(data[:4], magic[:]) {
		return nil, errors.New("not a .dct file (bad magic bytes)")
	}
	if data[4] != formatVersion {
		return nil, fmt.Errorf("unsupported format version %d", data[4])
	}

	h := &header{
		width:       int(binary.LittleEndian.Uint32(data[5:])),
		height:      int(binary.LittleEndian.Uint32(data[9:])),
		quality:     int(data[13]),
		subsampling: ChromaSubsampling(data[14]),
	}
	if h.width < 1 || h.height < 1 || h.width > maxDimension || h.height > maxDimension ||
		h.width*h.height > maxPixels {
		return nil, fmt.Errorf("invalid image size %dx%d", h.width, h.height)
	}
	if h.quality < 1 || h.quality > 100 {
		return nil, fmt.Errorf("invalid quality %d", h.quality)
	}
	if !h.subsampling.valid() {
		return nil, fmt.Errorf("invalid chroma subsampling mode %d", h.subsampling)
	}

	// Lengths are summed as uint64 so that three large 32-bit values cannot
	// overflow and slip past the bounds check.
	rest := data[headerSize:]
	var offset uint64
	for i := range h.streams {
		length := uint64(binary.LittleEndian.Uint32(data[15+4*i:]))
		if offset+length > uint64(len(rest)) {
			return nil, fmt.Errorf("data truncated: stream %d needs %d bytes, %d available",
				i, length, uint64(len(rest))-offset)
		}
		h.streams[i] = rest[offset : offset+length]
		offset += length
	}
	return h, nil
}

// decompressPlane rebuilds one plane from its integer stream: undo the DC
// delta coding and zero truncation, then dequantize and inverse-transform
// every block.
func decompressPlane(symbols []int, width, height int, table *block) (*plane, error) {
	blocksWide := ceilDiv(width, blockSize)
	blocksHigh := ceilDiv(height, blockSize)
	totalBlocks := blocksWide * blocksHigh

	// Pass 1 (sequential, because of the DC deltas): expand the stream into
	// 64 zigzag-ordered coefficients per block.
	quantized := make([]int, totalBlocks*64)
	pos, dc := 0, 0
	for b := 0; b < totalBlocks; b++ {
		if pos >= len(symbols) {
			return nil, fmt.Errorf("coefficient stream ends at block %d of %d", b, totalBlocks)
		}
		if symbols[pos] == eobMarker {
			return nil, fmt.Errorf("block %d has no DC coefficient", b)
		}
		dc += symbols[pos]
		pos++

		coeffs := quantized[b*64:][:64]
		coeffs[0] = dc
		for i := 1; ; i++ {
			if pos >= len(symbols) {
				return nil, fmt.Errorf("block %d has no end-of-block marker", b)
			}
			v := symbols[pos]
			pos++
			if v == eobMarker {
				break
			}
			if i > 63 {
				return nil, fmt.Errorf("block %d has more than 64 coefficients", b)
			}
			coeffs[i] = v
		}
	}
	if pos != len(symbols) {
		return nil, fmt.Errorf("%d unexpected symbols after the last block", len(symbols)-pos)
	}

	// Pass 2 (parallel): blocks no longer depend on each other.
	out := newPlane(width, height)
	forEachRow(blocksHigh, func(br int) {
		var q [blockSize][blockSize]int
		for bc := 0; bc < blocksWide; bc++ {
			coeffs := quantized[(br*blocksWide+bc)*64:][:64]
			for i, pos := range zigzag {
				q[pos/blockSize][pos%blockSize] = coeffs[i]
			}
			dequantized := dequantizeBlock(&q, table)
			samples := InverseDCT2D(dequantized)

			// Blocks on the right and bottom edge may extend past the
			// image; only the part inside it is kept.
			for r := 0; r < blockSize && br*blockSize+r < height; r++ {
				row := out.row(br*blockSize + r)
				for c := 0; c < blockSize && bc*blockSize+c < width; c++ {
					row[bc*blockSize+c] = samples[r][c]
				}
			}
		}
	})
	return out, nil
}

// upsample enlarges a color plane to width x height with bilinear
// interpolation: every output sample is a weighted average of the (up to)
// four nearest source samples. Compared with simply repeating each sample
// this avoids blocky color edges.
func upsample(src *plane, width, height int) *plane {
	type tap struct {
		lo, hi int     // the two source indices to blend
		weight float64 // share of hi; lo gets 1-weight
	}

	// taps maps each output index to its two source neighbours. Sample
	// centers are aligned: output i covers the same area as source
	// (i+0.5)*srcLen/dstLen - 0.5.
	taps := func(dstLen, srcLen int) []tap {
		out := make([]tap, dstLen)
		scale := float64(srcLen) / float64(dstLen)
		for i := range out {
			pos := (float64(i)+0.5)*scale - 0.5
			if pos < 0 {
				pos = 0
			}
			lo := int(math.Floor(pos))
			if lo > srcLen-1 {
				lo = srcLen - 1
			}
			hi := lo + 1
			if hi > srcLen-1 {
				hi = srcLen - 1
			}
			out[i] = tap{lo: lo, hi: hi, weight: pos - float64(lo)}
		}
		return out
	}

	// Horizontal and vertical weights are separable and identical for every
	// row or column, so they are computed once.
	xTaps := taps(width, src.width)
	yTaps := taps(height, src.height)

	dst := newPlane(width, height)
	forEachRow(height, func(r int) {
		yt := yTaps[r]
		top, bottom := src.row(yt.lo), src.row(yt.hi)
		out := dst.row(r)
		for c, xt := range xTaps {
			upper := top[xt.lo]*(1-xt.weight) + top[xt.hi]*xt.weight
			lower := bottom[xt.lo]*(1-xt.weight) + bottom[xt.hi]*xt.weight
			out[c] = upper*(1-yt.weight) + lower*yt.weight
		}
	})
	return dst
}

// toRGBA converts full-resolution Y, Cb and Cr planes back to an RGBA image.
func toRGBA(y, cb, cr *plane) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, y.width, y.height))

	forEachRow(y.height, func(r int) {
		dst := img.Pix[r*img.Stride:]
		yRow, cbRow, crRow := y.row(r), cb.row(r), cr.row(r)
		for c := 0; c < y.width; c++ {
			luma := yRow[c] + 128
			dst[4*c] = clampToByte(luma + 1.402*crRow[c])
			dst[4*c+1] = clampToByte(luma - 0.344136*cbRow[c] - 0.714136*crRow[c])
			dst[4*c+2] = clampToByte(luma + 1.772*cbRow[c])
			dst[4*c+3] = 255
		}
	})
	return img
}

// clampToByte rounds v to the nearest integer and limits it to [0, 255].
// Rounding matters: truncating would darken every pixel by half a level on
// average.
func clampToByte(v float64) uint8 {
	v += 0.5
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
