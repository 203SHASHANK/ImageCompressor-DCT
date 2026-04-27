# Phase 07 — Huffman Entropy Coding

> *After quantization, you have a stream of integers. Most are zero or near-zero; a few are large. Huffman coding assigns the shortest bit strings to the most common values, shrinking this stream by 60% or more.*

---

## What Is Entropy Coding?

Entropy coding is compression that exploits the statistical distribution of symbols. It assigns **shorter codes to more frequent symbols** and **longer codes to rarer symbols**.

### Simple Example

Suppose you have an integer stream: `[0, 0, 1, 0, -1, 0, 0, 2, 0, 0]`

Counting: `{0: 7, 1: 1, -1: 1, 2: 1}` — zero appears 7/10 times.

With fixed-width 8-bit integers, this takes 10 × 8 = 80 bits.

With Huffman coding: assign `0 → "0"` (1 bit), `1 → "100"` (3 bits), `-1 → "101"`, `2 → "110"`.

Encoded: `0 0 100 0 101 0 0 110 0 0` = 1+1+3+1+3+1+1+3+1+1 = 16 bits.

**Compression ratio: 80/16 = 5×.** The more skewed the distribution, the better the compression.

> **Real-World Analogy**: Morse code is a Huffman-like encoding. "E" (most common English letter) is a single dot (shortest). "Z" (rare) is dot-dot-dash-dash (longest). Same principle: frequent symbols → short codes.

---

## Why Per-Image Adaptive Huffman?

Standard JPEG uses **pre-defined static Huffman tables** derived from average image statistics. These tables work well for typical photographs but are suboptimal for:

- Synthetic images (screenshots, graphics) — different statistics
- Flat images (blue sky, white backgrounds) — almost all coefficients are zero
- Medical images, document scans — very different frequency distributions

DCTPress builds a **fresh Huffman tree for each image, for each channel (Y, Cb, Cr) independently**. This adapts to the actual coefficient distribution in this specific image.

The trade-off: the tree must be stored in the file header. For large images, the adaptive tree wins; for tiny images (< 100×100), the header overhead dominates.

DCTPress also encodes Y, Cb, and Cr with separate trees. Luma and chroma have different statistical distributions — Y has more varied coefficients; Cb/Cr are more heavily quantized and have more zeros. Separate trees adapt to each.

---

## `huffman/encoder.go` — Complete Walkthrough

### Data Structures

```go
type node struct {
    symbol    int   // the value this node represents (for leaves)
    frequency int   // how often this symbol appears
    left      *node
    right     *node
}
```

Each node in the binary tree holds a symbol (for leaves) and a frequency (for priority comparison). Internal nodes have `left` and `right` children; leaf nodes have `nil` children.

```go
type nodeHeap []*node

func (h nodeHeap) Len() int            { return len(h) }
func (h nodeHeap) Less(i, j int) bool  { return h[i].frequency < h[j].frequency }
func (h nodeHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x interface{}) { *h = append(*h, x.(*node)) }
func (h *nodeHeap) Pop() interface{}   { ... }
```

`nodeHeap` implements Go's `heap.Interface`. The interface requires: `Len()`, `Less()`, `Swap()`, `Push()`, `Pop()`. The `container/heap` package uses these methods to maintain the min-heap property.

`Less(i, j int) bool { return h[i].frequency < h[j].frequency }` — **min-heap** ordered by frequency (lowest frequency at root). This is the key: we always merge the two rarest nodes first.

```go
type CodeTable map[int]string
```

Maps each integer symbol to its Huffman code as a string of '0' and '1' characters. Example: `{0: "0", 1: "100", -1: "101", -32768: "111"}`.

---

## Building the Tree — `buildTree`

```go
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
        leftChild  := heap.Pop(minHeap).(*node)
        rightChild := heap.Pop(minHeap).(*node)
        heap.Push(minHeap, &node{
            frequency: leftChild.frequency + rightChild.frequency,
            left:      leftChild,
            right:     rightChild,
        })
    }

    return heap.Pop(minHeap).(*node)
}
```

**The Huffman algorithm step by step:**

1. Create a leaf node for each unique symbol, with frequency as priority
2. Insert all nodes into a min-heap
3. Repeat until one node remains:
   a. Pop the two lowest-frequency nodes
   b. Create a new internal node with frequency = sum of children's frequencies
   c. Push the internal node back into the heap
4. The remaining node is the root

**Complexity**: O(n log n) where n = number of unique symbols. Each heap operation is O(log n), and we do 2n heap operations.

### Worked Example

Symbols: `{0: 7, 1: 1, -1: 1, 2: 1}` (frequencies)

Initial heap: `[(−1,1), (1,1), (2,1), (0,7)]` (min-heap, smallest first)

**Iteration 1**: Pop (−1,1) and (1,1). Create internal node with freq=2. Push.
Heap: `[(2,1), (internal,2), (0,7)]`

**Iteration 2**: Pop (2,1) and (internal,2). Create internal with freq=3. Push.
Heap: `[(internal2,3), (0,7)]`

**Iteration 3**: Pop (internal2,3) and (0,7). Create root with freq=10. Push.
Heap: `[(root,10)]`

**Result tree**:
```
        root (10)
       /         \
  internal2 (3)   0 (7)
    /      \
internal (2)  2 (1)
  /   \
-1(1)  1(1)
```

**Codes** (left=0, right=1):
- `0` → "1" (1 bit, most frequent)
- `-1` → "000" (3 bits)
- `1` → "001" (3 bits)
- `2` → "01" (2 bits)

---

## Assigning Codes — `assignCodes`

```go
func assignCodes(current *node, prefix string, table CodeTable) {
    isLeaf := current.left == nil && current.right == nil
    if isLeaf {
        if prefix == "" {
            prefix = "0"  // single-symbol edge case
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
```

Recursive DFS through the tree. At each internal node, append "0" for the left branch and "1" for the right branch. When a leaf is reached, the accumulated prefix string is the code for that symbol.

The `if prefix == ""` case handles a single-symbol stream (only one unique symbol). You can't build a proper tree; the single node is both root and leaf. The code "0" is assigned by convention.

---

## The `Encode` Function — Wire Format

```go
func (encoder *Encoder) Encode(coefficients []int) ([]byte, error) {
    if len(coefficients) == 0 {
        return appendUint32(nil, 0), nil  // [symbolCount=0]
    }

    uniqueCount := len(countFrequencies(coefficients))

    // Single-symbol fast path
    if uniqueCount == 1 {
        sym := coefficients[0]
        buf := appendUint32(nil, 1)         // symbolCount = 1
        buf = appendInt16LE(buf, sym)       // the single symbol
        buf = appendUint32(buf, uint32(len(coefficients)))  // how many times
        return buf, nil
    }

    // General path
    treeNodes := serializeTree(encoder.root)

    var bitBuf []byte
    for _, coeff := range coefficients {
        code, exists := encoder.codeTable[coeff]
        if !exists {
            return nil, fmt.Errorf("coefficient %d not in code table", coeff)
        }
        bitBuf = append(bitBuf, []byte(code)...)  // append bit characters: '0' or '1'
    }
    packedBits, paddingBits := packBits(bitBuf)

    buf := appendUint32(nil, uint32(uniqueCount))
    buf = appendUint32(buf, uint32(len(treeNodes)))
    buf = append(buf, treeNodes...)
    buf = append(buf, byte(paddingBits))
    buf = append(buf, packedBits...)
    return buf, nil
}
```

**Single-symbol special case**: A flat image might produce a channel where every quantized coefficient is 0. Then `uniqueCount = 1`. Instead of building a degenerate tree (root = single leaf), we store just `[1][symbol][count]` — 9 bytes regardless of how many symbols. The decoder handles this by filling the output with `count` copies of `symbol`.

**Why store `uniqueCount`?** The decoder uses it to distinguish:
- `0` → empty channel
- `1` → single-symbol fast path
- `≥2` → full Huffman tree

**`bitBuf`**: Before packing, codes are stored as ASCII characters: each bit is a byte ('0' or '1'). This is simpler to build but wasteful — 1 byte per bit. `packBits` then compresses them into actual bits (1 bit per bit).

**`packedBits, paddingBits := packBits(bitBuf)`**: The bitstring might not be a multiple of 8 bits. `packBits` pads to the next byte boundary. The padding bit count (0–7) is stored so the decoder knows where the valid bits end.

---

## Tree Serialization — `serializeTree`

```go
func serializeTree(root *node) []byte {
    var buf []byte
    var walk func(n *node)
    walk = func(n *node) {
        if n.left == nil && n.right == nil {
            buf = append(buf, 'L')            // Leaf marker
            buf = appendInt16LE(buf, n.symbol) // 2-byte symbol
        } else {
            buf = append(buf, 'I')            // Internal marker
            if n.left != nil { walk(n.left) }
            if n.right != nil { walk(n.right) }
        }
    }
    walk(root)
    return buf
}
```

**Pre-order traversal**: visit node, then left subtree, then right subtree. This produces a serialization that can be unambiguously reconstructed.

Each node in the stream is:
- `'L' + int16(symbol)` = 3 bytes for a leaf
- `'I'` = 1 byte for an internal node

The tree structure is implicit: after each `'I'`, you recursively read the left child, then the right child.

**Example tree serialization** (from the worked example above):
```
root (internal)      → 'I'
  internal2 (int)    → 'I'
    internal (int)   → 'I'
      -1 (leaf)      → 'L' + [-1 as int16]
      1 (leaf)       → 'L' + [1 as int16]
    2 (leaf)         → 'L' + [2 as int16]
  0 (leaf)           → 'L' + [0 as int16]

Result bytes: I I I L FF FF L 01 00 L 02 00 L 00 00
```

(FF FF = -1 as signed int16 little-endian; 01 00 = 1; 02 00 = 2; 00 00 = 0)

---

## Tree Deserialization — `deserializeTree`

```go
func deserializeTree(data []byte, pos int) (*node, int, error) {
    flag := data[pos]
    pos++

    if flag == 'L' {
        sym := readInt16LE(data, pos)
        pos += 2
        return &node{symbol: sym}, pos, nil
    }

    if flag != 'I' {
        return nil, pos, fmt.Errorf("unknown node flag %q", flag)
    }

    leftChild, pos, err  := deserializeTree(data, pos)
    rightChild, pos, err := deserializeTree(data, pos)
    return &node{left: leftChild, right: rightChild}, pos, nil
}
```

Recursive descent, mirroring the pre-order serialization. The position `pos` is threaded through all calls — each call consumes exactly as many bytes as it needs and returns the updated position. This is a clean functional approach to stream parsing.

---

## Bit Packing

### `packBits` — ASCII bits to actual bits

```go
func packBits(bits []byte) ([]byte, int) {
    paddingBits := (8 - len(bits)%8) % 8
    packed := make([]byte, (len(bits)+paddingBits)/8)
    for i, bit := range bits {
        if bit == '1' {
            packed[i/8] |= 1 << (7 - uint(i%8))  // set bit MSB-first
        }
    }
    return packed, paddingBits
}
```

**MSB-first packing**: Bit 0 of the stream goes into the most significant bit of the first packed byte. This is the standard convention for bitstreams (network byte order for bits).

Example: bitstring `"10110"` (5 bits)
- paddingBits = (8 - 5%8) % 8 = 3
- packed byte: bit0='1'→bit7, bit1='0'→bit6, bit2='1'→bit5, bit3='1'→bit4, bit4='0'→bit3
- packed = `[10110000]` = 0xB0

### `unpackBits` — actual bits back to ASCII

```go
func unpackBits(packed []byte, paddingBits int) []byte {
    totalBits := len(packed)*8 - paddingBits
    bits := make([]byte, totalBits)
    for i := 0; i < totalBits; i++ {
        if packed[i/8] >> (7 - uint(i%8)) & 1 == 1 {
            bits[i] = '1'
        } else {
            bits[i] = '0'
        }
    }
    return bits
}
```

Reads `totalBits = len(packed)*8 - paddingBits` bits, stopping before the padding. For each bit position `i`, tests the corresponding bit in the packed byte array.

---

## Symbol Decoding — `decodeSymbols`

```go
func decodeSymbols(root *node, bits []byte) ([]int, error) {
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
        if current.left == nil && current.right == nil {  // leaf reached
            symbols = append(symbols, current.symbol)
            current = root  // reset to start of tree for next symbol
        }
    }
    return symbols, nil
}
```

The decoding traversal:
1. Start at root
2. For each bit: go left (0) or right (1)
3. When a leaf is reached: emit the symbol, reset to root
4. Repeat

**Why this works**: Huffman codes are **prefix-free** — no valid code is a prefix of another. This means whenever you reach a leaf, you know the current code is complete. You don't need any length information or delimiters.

**`make([]int, 0, len(bits)/4)`**: Pre-size estimate. Average bit length per symbol is 2–5 bits for typical distributions; `len(bits)/4` is a reasonable middle-ground pre-allocation.

---

## Integer Encoding Helpers

```go
func appendInt16LE(buf []byte, v int) []byte {
    u := uint16(int16(v))          // convert to int16, then to uint16
    return append(buf, byte(u), byte(u>>8))
}

func readInt16LE(data []byte, offset int) int {
    u := uint16(data[offset]) | uint16(data[offset+1])<<8
    return int(int16(u))           // interpret as signed int16
}
```

DCT coefficients are signed integers (positive and negative). They are stored as signed 16-bit little-endian integers. The range is [-32768, 32767].

- `int16(v)` — truncate to signed 16-bit
- `uint16(int16(v))` — reinterpret the bit pattern as unsigned (for byte manipulation)
- `int(int16(u))` on decode — reinterpret the 16-bit unsigned as signed, then extend to int

The EOB marker `-32768 = math.MinInt16` is a valid int16 value (the most negative) — this is exactly why it was chosen as the sentinel.

---

## Compression Effect of Huffman

For a 1080p image at quality=75, the quantized coefficient stream (before Huffman) might have:
- ~15 million integers for Y channel
- Most are 0 (or near-zero small integers)
- As raw int16: 30 MB

After Huffman:
- Zero gets a 1-bit code (0 is extremely common)
- Small ±1, ±2 values get 3–5 bit codes
- Rare large values get 8–12 bit codes
- Typical result: ~80 KB for Y channel (≈ **375× compression** from raw int16)

Combined with quantization, the full pipeline achieves 48× from raw RGB.

---

## What Didn't Work — Run-Length AC Encoding

The README documents:

> "Run-length encoding of AC coefficients — storing (zeroRun, value) pairs — made things worse: file size increased from ~97 KB to ~134 KB."

Standard JPEG encodes AC coefficients as `(numZerosBefore, coefficient)` pairs. This avoids writing individual zeros.

But DCTPress's adaptive Huffman already assigns a 1-bit code to zero (the most common symbol). Replacing runs of zeros with `(runLength, 0)` pairs:
1. Adds a symbol for `runLength` that Huffman must now also encode
2. Removes the opportunity for Huffman to give zero its optimal 1-bit code
3. For dense blocks (many non-zero ACs), run-length pairs add more overhead than they save

The EOB marker approach (DCTPress's choice) is simpler and works better with the adaptive Huffman: zeros in the middle of the sequence are coded as individual 1-bit '0' symbols; zeros at the end are not stored at all.

---

*Next: [Phase 08 — Quality Metrics: PSNR and SSIM](08_quality_metrics.md)*
