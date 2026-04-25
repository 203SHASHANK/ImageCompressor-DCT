// Package huffman implements entropy coding for quantized DCT coefficients.
// It uses a Huffman tree built from symbol frequencies, producing a compact
// bitstream. Single-symbol inputs are handled as a special case.
package huffman

import (
	"container/heap"
	"fmt"
)

// node is a single node in the Huffman binary tree.
type node struct {
	symbol    int
	frequency int
	left      *node
	right     *node
}

// nodeHeap is a min-heap of *node ordered by ascending frequency.
type nodeHeap []*node

func (h nodeHeap) Len() int            { return len(h) }
func (h nodeHeap) Less(i, j int) bool  { return h[i].frequency < h[j].frequency }
func (h nodeHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x interface{}) { *h = append(*h, x.(*node)) }
func (h *nodeHeap) Pop() interface{} {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

// CodeTable maps each symbol to its Huffman bit-string (e.g. "010").
type CodeTable map[int]string

// Encoder builds a Huffman tree from a coefficient stream and encodes it.
type Encoder struct {
	root      *node
	codeTable CodeTable
}

// NewEncoder analyses coefficient frequencies and builds the Huffman tree.
func NewEncoder(coefficients []int) *Encoder {
	frequencies := countFrequencies(coefficients)
	root := buildTree(frequencies)
	codeTable := make(CodeTable)
	if root != nil {
		assignCodes(root, "", codeTable)
	}
	return &Encoder{root: root, codeTable: codeTable}
}

// Encode converts a coefficient slice into a self-contained byte slice.
//
// Wire format:
//
//	[symbolCount(4)] — number of unique symbols (0 = empty)
//	if symbolCount == 1:
//	  [symbol(2 signed LE)][inputLen(4)] — repeat that symbol inputLen times
//	else:
//	  [treeNodeCount(4)][serialised tree nodes...][paddingBits(1)][packed bitstream...]
//
// Tree node format (pre-order, each node = 1 flag byte):
//
//	'L' + [symbol(2 signed LE)]  — leaf
//	'I'                          — internal node (left child follows, then right)
func (encoder *Encoder) Encode(coefficients []int) ([]byte, error) {
	if len(coefficients) == 0 {
		return appendUint32(nil, 0), nil
	}

	uniqueCount := len(countFrequencies(coefficients))

	// ── Single-symbol fast path ────────────────────────────────────────────
	if uniqueCount == 1 {
		sym := coefficients[0]
		buf := appendUint32(nil, 1)
		buf = appendInt16LE(buf, sym)
		buf = appendUint32(buf, uint32(len(coefficients)))
		return buf, nil
	}

	// ── General path ──────────────────────────────────────────────────────
	// Serialise tree.
	treeNodes := serializeTree(encoder.root)

	// Build bitstream.
	var bitBuf []byte
	for _, coeff := range coefficients {
		code, exists := encoder.codeTable[coeff]
		if !exists {
			return nil, fmt.Errorf("coefficient %d not in code table", coeff)
		}
		bitBuf = append(bitBuf, []byte(code)...)
	}
	packedBits, paddingBits := packBits(bitBuf)

	buf := appendUint32(nil, uint32(uniqueCount))
	buf = appendUint32(buf, uint32(len(treeNodes)))
	buf = append(buf, treeNodes...)
	buf = append(buf, byte(paddingBits))
	buf = append(buf, packedBits...)
	return buf, nil
}

// Decode reconstructs the original coefficient slice from Huffman-encoded bytes.
func Decode(data []byte) ([]int, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("huffman data too short (%d bytes)", len(data))
	}

	uniqueCount := int(readUint32(data, 0))

	if uniqueCount == 0 {
		return []int{}, nil
	}

	// ── Single-symbol fast path ────────────────────────────────────────────
	if uniqueCount == 1 {
		if len(data) < 4+2+4 {
			return nil, fmt.Errorf("single-symbol data too short")
		}
		sym := readInt16LE(data, 4)
		count := int(readUint32(data, 6))
		result := make([]int, count)
		for i := range result {
			result[i] = sym
		}
		return result, nil
	}

	// ── General path ──────────────────────────────────────────────────────
	if len(data) < 8 {
		return nil, fmt.Errorf("huffman data too short for tree header")
	}
	nodeCount := int(readUint32(data, 4))
	treeEnd := 8 + nodeCount
	if len(data) < treeEnd+1 {
		return nil, fmt.Errorf("huffman data truncated at tree (need %d bytes)", treeEnd+1)
	}

	treeNodes := data[8:treeEnd]
	root, _, err := deserializeTree(treeNodes, 0)
	if err != nil {
		return nil, fmt.Errorf("tree deserialisation failed: %w", err)
	}

	paddingBits := int(data[treeEnd])
	packedBits := data[treeEnd+1:]

	bitString := unpackBits(packedBits, paddingBits)
	return decodeSymbols(root, bitString)
}

// ── Tree construction ──────────────────────────────────────────────────────

func countFrequencies(coefficients []int) map[int]int {
	frequencies := make(map[int]int, 512)
	for _, coeff := range coefficients {
		frequencies[coeff]++
	}
	return frequencies
}

func buildTree(frequencies map[int]int) *node {
	if len(frequencies) == 0 {
		return nil
	}

	minHeap := &nodeHeap{}
	heap.Init(minHeap)
	for symbol, freq := range frequencies {
		heap.Push(minHeap, &node{symbol: symbol, frequency: freq})
	}

	for minHeap.Len() > 1 {
		leftChild := heap.Pop(minHeap).(*node)
		rightChild := heap.Pop(minHeap).(*node)
		heap.Push(minHeap, &node{
			frequency: leftChild.frequency + rightChild.frequency,
			left:      leftChild,
			right:     rightChild,
		})
	}

	return heap.Pop(minHeap).(*node)
}

func assignCodes(current *node, prefix string, table CodeTable) {
	isLeaf := current.left == nil && current.right == nil
	if isLeaf {
		if prefix == "" {
			prefix = "0"
		}
		table[current.symbol] = prefix
		return
	}
	if current.left != nil {
		assignCodes(current.left, prefix+"0", table)
	}
	if current.right != nil {
		assignCodes(current.right, prefix+"1", table)
	}
}

// ── Tree serialisation ─────────────────────────────────────────────────────
// Each node is one byte: 'L' (leaf, followed by 2-byte symbol) or 'I' (internal).
// Nodes are written in pre-order; leaves carry their symbol inline.

func serializeTree(root *node) []byte {
	if root == nil {
		return nil
	}
	var buf []byte
	var walk func(n *node)
	walk = func(n *node) {
		if n.left == nil && n.right == nil {
			buf = append(buf, 'L')
			buf = appendInt16LE(buf, n.symbol)
		} else {
			buf = append(buf, 'I')
			if n.left != nil {
				walk(n.left)
			}
			if n.right != nil {
				walk(n.right)
			}
		}
	}
	walk(root)
	return buf
}

// deserializeTree reconstructs a tree from the serialised node bytes.
// Returns the root node and the number of bytes consumed.
func deserializeTree(data []byte, pos int) (*node, int, error) {
	if pos >= len(data) {
		return nil, pos, fmt.Errorf("unexpected end of tree data at position %d", pos)
	}
	flag := data[pos]
	pos++

	if flag == 'L' {
		if pos+2 > len(data) {
			return nil, pos, fmt.Errorf("truncated leaf symbol at position %d", pos)
		}
		sym := readInt16LE(data, pos)
		pos += 2
		return &node{symbol: sym}, pos, nil
	}

	if flag != 'I' {
		return nil, pos, fmt.Errorf("unknown node flag %q at position %d", flag, pos-1)
	}

	leftChild, pos, err := deserializeTree(data, pos)
	if err != nil {
		return nil, pos, err
	}
	rightChild, pos, err := deserializeTree(data, pos)
	if err != nil {
		return nil, pos, err
	}
	return &node{left: leftChild, right: rightChild}, pos, nil
}

// ── Symbol decoding ────────────────────────────────────────────────────────

func decodeSymbols(root *node, bits []byte) ([]int, error) {
	if root == nil {
		return nil, nil
	}
	symbols := make([]int, 0, len(bits)/4)
	current := root
	for _, bit := range bits {
		if bit == '0' {
			current = current.left
		} else {
			current = current.right
		}
		if current == nil {
			return nil, fmt.Errorf("invalid bit sequence: reached nil node")
		}
		if current.left == nil && current.right == nil {
			symbols = append(symbols, current.symbol)
			current = root
		}
	}
	return symbols, nil
}

// ── Bit packing ────────────────────────────────────────────────────────────

func packBits(bits []byte) ([]byte, int) {
	if len(bits) == 0 {
		return []byte{}, 0
	}
	paddingBits := (8 - len(bits)%8) % 8
	packed := make([]byte, (len(bits)+paddingBits)/8)
	for i, bit := range bits {
		if bit == '1' {
			packed[i/8] |= 1 << (7 - uint(i%8))
		}
	}
	return packed, paddingBits
}

func unpackBits(packed []byte, paddingBits int) []byte {
	totalBits := len(packed)*8 - paddingBits
	if totalBits <= 0 {
		return []byte{}
	}
	bits := make([]byte, totalBits)
	for i := 0; i < totalBits; i++ {
		if packed[i/8]>>(7-uint(i%8))&1 == 1 {
			bits[i] = '1'
		} else {
			bits[i] = '0'
		}
	}
	return bits
}

// ── Byte helpers ───────────────────────────────────────────────────────────

func appendUint32(buf []byte, v uint32) []byte {
	return append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func readUint32(data []byte, offset int) uint32 {
	return uint32(data[offset]) |
		uint32(data[offset+1])<<8 |
		uint32(data[offset+2])<<16 |
		uint32(data[offset+3])<<24
}

func appendInt16LE(buf []byte, v int) []byte {
	u := uint16(int16(v))
	return append(buf, byte(u), byte(u>>8))
}

func readInt16LE(data []byte, offset int) int {
	u := uint16(data[offset]) | uint16(data[offset+1])<<8
	return int(int16(u))
}
