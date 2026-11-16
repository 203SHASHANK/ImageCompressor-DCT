// Package huffman implements Huffman entropy coding for streams of small
// signed integers, such as quantized DCT coefficients.
//
// Huffman coding gives every distinct value a bit pattern whose length
// depends on how often the value occurs: frequent values get short codes and
// rare ones long codes. The code is built per input, and the tree describing
// it is stored in front of the data so the decoder can rebuild it.
//
// Wire format (all integers little-endian):
//
//	symbolCount uint32               number of distinct symbols
//	symbolCount == 0:                nothing follows (empty input)
//	symbolCount == 1:
//	    symbol  int16                the only symbol
//	    repeat  uint32               how many times it occurs
//	symbolCount >= 2:
//	    treeLen uint32               size of the serialized tree in bytes
//	    tree    [treeLen]byte        pre-order: 'I' for an internal node,
//	                                 'L' + int16 symbol for a leaf
//	    padding uint8                unused bits in the final data byte (0-7)
//	    data    []byte               codes, most significant bit first
package huffman

import (
	"container/heap"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	// Symbols are stored as int16 in the serialized tree.
	minSymbol = math.MinInt16
	maxSymbol = math.MaxInt16

	leafTag     = 'L'
	internalTag = 'I'

	// maxCodeLength is the longest code the bit writer can hold. A Huffman
	// code this long needs an input of more than 2^44 symbols, so it cannot
	// occur for data that fits in memory; it is checked all the same.
	maxCodeLength = 56

	// maxTreeDepth bounds recursion when reading a tree from untrusted data.
	maxTreeDepth = 1024

	// defaultMaxSymbols is the output limit Decode applies.
	defaultMaxSymbols = 1 << 26
)

// ErrSymbolRange is returned when an input value does not fit in an int16.
var ErrSymbolRange = errors.New("huffman: symbol outside the int16 range")

// node is one node of the Huffman tree. Leaves have no children.
type node struct {
	symbol      int
	frequency   int
	order       int // creation sequence number; makes heap ordering total
	left, right *node
}

func (n *node) isLeaf() bool { return n.left == nil && n.right == nil }

// nodeHeap is a min-heap of nodes by frequency. Ties are broken by creation
// order so that the same input always produces the same tree, and therefore
// byte-identical output.
type nodeHeap []*node

func (h nodeHeap) Len() int { return len(h) }
func (h nodeHeap) Less(i, j int) bool {
	if h[i].frequency != h[j].frequency {
		return h[i].frequency < h[j].frequency
	}
	return h[i].order < h[j].order
}
func (h nodeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)   { *h = append(*h, x.(*node)) }
func (h *nodeHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

// code is the bit pattern assigned to one symbol, right-aligned in bits.
type code struct {
	bits   uint64
	length uint8 // 0 means the symbol has no code
}

// Encoder holds the Huffman code built for one input.
type Encoder struct {
	root        *node
	symbolCount int

	// codes is indexed by symbol-base, covering only the range of symbols
	// actually present. A slice lookup is much cheaper than a map lookup in
	// the per-symbol encoding loop.
	codes []code
	base  int

	err error // construction failure, reported by Encode
}

// NewEncoder counts symbol frequencies in coefficients and builds the
// Huffman code for them. Any error (a symbol that does not fit in int16) is
// reported by Encode.
func NewEncoder(coefficients []int) *Encoder {
	e := &Encoder{}
	if len(coefficients) == 0 {
		return e
	}

	lo, hi := coefficients[0], coefficients[0]
	for _, c := range coefficients {
		if c < lo {
			lo = c
		}
		if c > hi {
			hi = c
		}
	}
	if lo < minSymbol || hi > maxSymbol {
		e.err = fmt.Errorf("%w: input spans [%d, %d]", ErrSymbolRange, lo, hi)
		return e
	}

	counts := make([]int, hi-lo+1)
	for _, c := range coefficients {
		counts[c-lo]++
	}

	e.base = lo
	e.root, e.symbolCount = buildTree(counts, lo)
	e.codes = make([]code, len(counts))
	if e.symbolCount > 1 {
		e.err = assignCodes(e.root, 0, 0, e.codes, lo)
	}
	return e
}

// Encode compresses coefficients with the code built by NewEncoder. Every
// value in coefficients must have been present in the slice passed to
// NewEncoder; normally the same slice is passed to both.
func (e *Encoder) Encode(coefficients []int) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	if len(coefficients) == 0 {
		return binary.LittleEndian.AppendUint32(nil, 0), nil
	}
	if e.symbolCount == 0 {
		return nil, errors.New("huffman: encoder was built from empty input")
	}

	// A single distinct symbol needs no tree and no bits: store the symbol
	// and how often it repeats.
	if e.symbolCount == 1 {
		symbol := e.root.symbol
		for _, c := range coefficients {
			if c != symbol {
				return nil, fmt.Errorf("huffman: symbol %d has no code", c)
			}
		}
		buf := binary.LittleEndian.AppendUint32(nil, 1)
		buf = binary.LittleEndian.AppendUint16(buf, uint16(int16(symbol)))
		return binary.LittleEndian.AppendUint32(buf, uint32(len(coefficients))), nil
	}

	tree := serializeTree(e.root, make([]byte, 0, e.symbolCount*4))

	buf := make([]byte, 0, 9+len(tree)+len(coefficients)/2)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(e.symbolCount))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(tree)))
	buf = append(buf, tree...)
	paddingAt := len(buf)
	buf = append(buf, 0) // padding count, filled in below

	// Pack codes into bytes through a 64-bit accumulator. Whole bytes are
	// flushed from the top as soon as they are complete, so at most 7 bits
	// are pending when the next code (up to maxCodeLength bits) is added.
	var acc uint64
	var pending uint
	for _, c := range coefficients {
		idx := c - e.base
		if idx < 0 || idx >= len(e.codes) || e.codes[idx].length == 0 {
			return nil, fmt.Errorf("huffman: symbol %d has no code", c)
		}
		cd := e.codes[idx]
		acc = acc<<cd.length | cd.bits
		pending += uint(cd.length)
		for pending >= 8 {
			pending -= 8
			buf = append(buf, byte(acc>>pending))
		}
	}
	if pending > 0 {
		padding := 8 - pending
		buf = append(buf, byte(acc<<padding))
		buf[paddingAt] = byte(padding)
	}
	return buf, nil
}

// Decode reverses Encoder.Encode. It refuses inputs that would expand to
// more than 2^26 symbols; use DecodeLimit to set a tighter bound when the
// expected size is known.
func Decode(data []byte) ([]int, error) {
	return DecodeLimit(data, defaultMaxSymbols)
}

// DecodeLimit is Decode with a caller-chosen cap on the number of decoded
// symbols. The input is treated as untrusted: the cap stops a few crafted
// bytes from forcing a huge allocation.
func DecodeLimit(data []byte, maxSymbols int) ([]int, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("huffman: data too short (%d bytes)", len(data))
	}
	symbolCount := binary.LittleEndian.Uint32(data)

	switch symbolCount {
	case 0:
		return []int{}, nil

	case 1:
		if len(data) < 10 {
			return nil, errors.New("huffman: single-symbol data too short")
		}
		symbol := int(int16(binary.LittleEndian.Uint16(data[4:])))
		repeat := binary.LittleEndian.Uint32(data[6:])
		if uint64(repeat) > uint64(maxSymbols) {
			return nil, fmt.Errorf("huffman: %d symbols exceeds the limit of %d", repeat, maxSymbols)
		}
		out := make([]int, repeat)
		for i := range out {
			out[i] = symbol
		}
		return out, nil
	}

	if len(data) < 8 {
		return nil, errors.New("huffman: data too short for tree header")
	}
	treeLen := uint64(binary.LittleEndian.Uint32(data[4:]))
	treeEnd := 8 + treeLen
	if uint64(len(data)) < treeEnd+1 {
		return nil, fmt.Errorf("huffman: data truncated inside tree (need %d bytes)", treeEnd+1)
	}

	root, used, leaves, err := deserializeTree(data[8:treeEnd], 0, 0)
	if err != nil {
		return nil, err
	}
	if uint64(used) != treeLen {
		return nil, errors.New("huffman: trailing bytes after tree")
	}
	if root.isLeaf() || uint32(leaves) != symbolCount {
		return nil, errors.New("huffman: tree does not match symbol count")
	}

	padding := int(data[treeEnd])
	packed := data[treeEnd+1:]
	if padding > 7 || (len(packed) == 0 && padding != 0) {
		return nil, fmt.Errorf("huffman: invalid padding %d", padding)
	}
	totalBits := len(packed)*8 - padding

	// Every code is at least one bit long, which bounds the output size.
	capacity := totalBits
	if capacity > maxSymbols {
		capacity = maxSymbols
	}
	out := make([]int, 0, capacity/2+1)

	// Walk the tree one bit at a time: 0 goes left, 1 goes right, and
	// reaching a leaf emits its symbol and restarts from the root.
	current := root
	for i := 0; i < totalBits; i++ {
		if packed[i>>3]>>(7-uint(i&7))&1 == 0 {
			current = current.left
		} else {
			current = current.right
		}
		if current.isLeaf() {
			if len(out) >= maxSymbols {
				return nil, fmt.Errorf("huffman: output exceeds the limit of %d symbols", maxSymbols)
			}
			out = append(out, current.symbol)
			current = root
		}
	}
	if current != root {
		return nil, errors.New("huffman: data ends in the middle of a code")
	}
	return out, nil
}

// buildTree builds the Huffman tree for the given counts, where counts[i] is
// the frequency of symbol base+i. It returns the root and the number of
// distinct symbols.
//
// The algorithm repeatedly removes the two least frequent nodes and joins
// them under a new parent whose frequency is their sum. Rare symbols are
// merged early and so end up deep in the tree, with long codes.
func buildTree(counts []int, base int) (*node, int) {
	h := make(nodeHeap, 0, len(counts))
	order := 0
	for i, count := range counts {
		if count > 0 {
			h = append(h, &node{symbol: base + i, frequency: count, order: order})
			order++
		}
	}
	leaves := len(h)
	if leaves == 0 {
		return nil, 0
	}

	heap.Init(&h)
	for h.Len() > 1 {
		left := heap.Pop(&h).(*node)
		right := heap.Pop(&h).(*node)
		heap.Push(&h, &node{
			frequency: left.frequency + right.frequency,
			order:     order,
			left:      left,
			right:     right,
		})
		order++
	}
	return h[0], leaves
}

// assignCodes walks the tree and records the path to every leaf as its
// code: a 0 bit for each step left, a 1 bit for each step right.
func assignCodes(n *node, bits uint64, length uint8, codes []code, base int) error {
	if n.isLeaf() {
		codes[n.symbol-base] = code{bits: bits, length: length}
		return nil
	}
	if length >= maxCodeLength {
		return fmt.Errorf("huffman: code longer than %d bits", maxCodeLength)
	}
	if err := assignCodes(n.left, bits<<1, length+1, codes, base); err != nil {
		return err
	}
	return assignCodes(n.right, bits<<1|1, length+1, codes, base)
}

// serializeTree appends the tree to buf in pre-order.
func serializeTree(n *node, buf []byte) []byte {
	if n.isLeaf() {
		buf = append(buf, leafTag)
		return binary.LittleEndian.AppendUint16(buf, uint16(int16(n.symbol)))
	}
	buf = append(buf, internalTag)
	buf = serializeTree(n.left, buf)
	return serializeTree(n.right, buf)
}

// deserializeTree reads one subtree starting at data[pos]. It returns the
// subtree, the position just past it, and the number of leaves it contains.
func deserializeTree(data []byte, pos, depth int) (n *node, next, leaves int, err error) {
	if depth > maxTreeDepth {
		return nil, pos, 0, errors.New("huffman: tree too deep")
	}
	if pos >= len(data) {
		return nil, pos, 0, errors.New("huffman: tree data ends early")
	}

	switch data[pos] {
	case leafTag:
		if pos+3 > len(data) {
			return nil, pos, 0, errors.New("huffman: truncated leaf")
		}
		symbol := int(int16(binary.LittleEndian.Uint16(data[pos+1:])))
		return &node{symbol: symbol}, pos + 3, 1, nil

	case internalTag:
		left, next, leftLeaves, err := deserializeTree(data, pos+1, depth+1)
		if err != nil {
			return nil, next, 0, err
		}
		right, next, rightLeaves, err := deserializeTree(data, next, depth+1)
		if err != nil {
			return nil, next, 0, err
		}
		return &node{left: left, right: right}, next, leftLeaves + rightLeaves, nil
	}
	return nil, pos, 0, fmt.Errorf("huffman: unknown node tag 0x%02x", data[pos])
}
