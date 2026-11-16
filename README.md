# ImageCompressor-DCT

A JPEG-style lossy image codec written from scratch in Go: block DCT,
quantization, zigzag ordering and Huffman coding, with no codec libraries.
It comes with a decoder, a command-line tool, an HTTP API and a web UI that
benchmarks it against Go's standard JPEG encoder, Pillow and OpenCV.

The codec is called DCTPress in the UI and the benchmark output. Its file
format (`.dct`) is its own and is not JPEG-compatible.

## Quick start

Requires Go 1.25+. Python 3 with Pillow and OpenCV is optional and only adds
two rows to the benchmark.

```bash
git clone https://github.com/203SHASHANK/ImageCompressor-DCT
cd ImageCompressor-DCT

make run            # web UI and API on http://localhost:8081
make test           # unit and integration tests
```

Command line:

```bash
make build-cli
./bin/dctpress encode -q 75 photo.png photo.dct
./bin/dctpress decode photo.dct roundtrip.png
./bin/dctpress bench  -q 75 photo.png
```

Docker:

```bash
docker compose up   # http://localhost:8080
```

## Results

Four 1920x1080 photographs at quality 75 with 4:2:0 chroma subsampling.
File size and PSNR, DCTPress against Go's `image/jpeg`:

| Photo | DCTPress | Go stdlib JPEG | Size difference | PSNR (DCTPress / stdlib) |
|---|---|---|---|---|
| Flower | 95.4 KB | 95.5 KB | -0.2% | 43.00 / 41.51 dB |
| Mirror | 138.0 KB | 141.6 KB | -2.5% | 40.70 / 39.87 dB |
| Tree | 154.1 KB | 146.1 KB | +5.5% | 39.03 / 38.13 dB |
| Landscape | 295.2 KB | 282.7 KB | +4.4% | 32.73 / 32.40 dB |

In short: file sizes within about 5% of the standard library encoder in
either direction, at 0.3 to 1.5 dB higher PSNR. Against raw 24-bit RGB that
is a 21x to 65x reduction, depending on how much detail the photo has.

Full output for one photo, including the Python encoders:

```
codec                 bytes    ratio   PSNR dB     SSIM  encode ms  decode ms
DCTPress             138004    45.1x     40.70   0.9670       60.2       85.1
Go stdlib JPEG       141577    43.9x     39.87   0.9657      106.1       43.7
Python PIL           131119    47.4x     40.43   0.9660       37.4       23.3
Python OpenCV        141290    44.0x     40.43   0.9660       12.0       20.4
```

How to read this:

- Every codec is measured the same way. Size is compared against raw RGB,
  and PSNR and SSIM are computed by this program from each codec's own
  decoded output.
- Pillow and OpenCV call libjpeg-turbo, which is hand-optimized C with SIMD.
  They are faster, and Pillow with optimized Huffman tables is also smaller.
  They are the reference, not the competition.
- Timings are medians of five runs on a 12-thread laptop CPU and vary with
  load. The Python rows time only the encode and decode calls, not
  interpreter start-up.

Reproduce with `./bin/dctpress bench <image>`.

### Speed of the building blocks

From `make bench`, same machine:

| Benchmark | Result |
|---|---|
| 8x8 DCT, direct formula | ~190,000 ns |
| 8x8 DCT, separable with precomputed basis | ~1,000 ns |
| Encode 1080p photo, one core | ~170 ms |
| Encode 1080p photo, worker pool on 12 threads | ~75 ms |
| Decode 1080p photo | ~85 ms |

The worker pool gives about 2.2x, not 12x, because only the per-block work
(color conversion, DCT, quantization) is parallel. Delta coding and Huffman
coding are sequential within a channel.

## How the codec works

Encoding (`internal/core/dct/encoder.go`):

1. **Color conversion.** RGB is converted to YCbCr, which separates
   brightness (Y) from color (Cb, Cr).
2. **Chroma subsampling.** The eye resolves color detail poorly, so the two
   color planes can be stored at half resolution: 4:2:0 halves both
   directions, 4:2:2 only the horizontal, 4:4:4 keeps everything.
3. **8x8 blocks and DCT.** Each plane is cut into 8x8 blocks. The discrete
   cosine transform rewrites a block as 64 frequency coefficients: one
   average (DC) and 63 patterns of increasing detail (AC). For photographs
   most of the energy lands in a few low-frequency coefficients.
4. **Quantization.** Each coefficient is divided by a step size from the
   JPEG standard tables, scaled by the quality setting, and rounded. This is
   the only lossy step. High frequencies get large steps and mostly round to
   zero.
5. **Zigzag ordering.** Coefficients are read out diagonally from low to
   high frequency, so the zeros collect at the end of each block.
6. **DC delta and end-of-block.** The DC value is stored as the difference
   from the previous block, and everything after the last non-zero
   coefficient is replaced by a single end-of-block marker.
7. **Huffman coding.** The resulting integers are entropy-coded with a
   Huffman code built for this image and channel, so common values (zero,
   the end-of-block marker, small numbers) take one to three bits.

Decoding reverses the steps. Subsampled color planes are scaled back up with
bilinear interpolation.

Differences from baseline JPEG:

- Huffman tables are built per image and channel and stored in the file,
  where baseline JPEG encoders usually ship fixed tables.
- Symbols are whole coefficient values. JPEG codes (zero-run, size) pairs
  followed by raw bits, which is more compact for long runs of zeros.
- The DC quantization step is 20% finer than the standard table.

### File format

```
offset  size  field
0       4     magic "DCT\x02"
4       1     format version (2)
5       4     width, little-endian
9       4     height, little-endian
13      1     quality (1-100)
14      1     chroma subsampling (0 = 4:4:4, 1 = 4:2:2, 2 = 4:2:0)
15      12    byte lengths of the Y, Cb and Cr streams (3 x uint32)
27      ...   the three Huffman-coded streams
```

Quantization tables are not stored: the decoder rebuilds them from the
quality byte. Each stream starts with its serialized Huffman tree; the
layout is documented in `internal/core/huffman/huffman.go`.

The decoder treats its input as untrusted. Header fields, stream lengths,
tree shape and symbol counts are all validated, and both decoders have fuzz
targets (`make fuzz`).

## Quality metrics

- **PSNR** (`internal/core/metrics/psnr.go`): `10 * log10(255^2 / MSE)` over
  all R, G and B samples. Higher is better; 30 to 40 dB is typical for lossy
  photo compression.
- **SSIM** (`internal/core/metrics/ssim.go`): structural similarity on
  brightness, using uniform 8x8 windows moved in steps of 4. It follows the
  formula from Wang et al. (2004) but not the reference 11x11 Gaussian
  window, so values are close to, not identical with, other tools.

## HTTP API

| Method and path | Purpose |
|---|---|
| `POST /api/upload` | Multipart form, field `image`. Returns an image ID and metadata. |
| `POST /api/compress` | JSON `{"image_id", "quality", "chroma_subsampling"}`. Returns sizes, timings, PSNR, SSIM and result URLs. |
| `POST /api/benchmark` | JSON `{"image_id", "quality"}`. Runs every available codec. |
| `GET /api/download/{id}` | A stored result: the `.dct` file or the preview. |
| `GET /api/export?id=&format=` | The result as `dct`, `jpeg` or `png`. |
| `GET /api/formats` | Accepted input and output formats. |
| `GET /health` | Liveness, version, and whether Python was found. |

Example:

```bash
curl -F image=@photo.png localhost:8081/api/upload
# {"id":"img_356633d5...","format":"png","width":1920,"height":1080,...}

curl -X POST localhost:8081/api/compress \
  -d '{"image_id":"img_356633d5...","quality":75,"chroma_subsampling":"4:2:0"}'
```

Input formats: PNG, JPEG, WebP, BMP, TIFF.

Limits: uploads up to 10 MB and 16 megapixels. Uploads and results are kept
in memory for 30 minutes, with a cap on how many are held, and are addressed
by random IDs. There is no authentication; this is a demo server, not a
hosted service.

## Web UI

Served at the root URL:

- drag-and-drop upload, quality slider and subsampling selector
- original, decoded and difference-heatmap views side by side
- size, ratio, PSNR, SSIM and timing for each compression
- benchmark table across all available codecs
- export as `.dct`, JPEG or PNG

Screenshots are in [Gallery/](Gallery/).

## Project layout

```
cmd/server/            HTTP server
cmd/dctpress/          command-line encoder, decoder and benchmark
internal/core/dct/     transform, quantization, encoder, decoder, file format
internal/core/huffman/ Huffman coder
internal/core/metrics/ PSNR and SSIM
internal/core/pixels/  fast pixel access shared by codec and metrics
internal/handlers/     HTTP handlers and the in-memory stores
internal/services/     compression and benchmark orchestration
internal/python/       subprocess bridge and the Pillow / OpenCV scripts
test/integration/      end-to-end API tests
web/                   static single-page UI
```

## Development

```bash
make test         # all tests with coverage
make test-race    # with the race detector
make bench        # microbenchmarks
make fuzz         # fuzz both decoders for 30 s each
make lint         # golangci-lint
make setup        # Go modules and the optional Python packages
```

## Design notes

**Why separable DCT rather than a fast butterfly algorithm?** Two passes of
an 8-point transform with a precomputed basis table are easy to verify
against the definition (there is a test that does exactly that) and already
about 190x faster than the direct formula. A scaled integer algorithm such
as AAN would be faster again, at the cost of fixed-point care.

**Why per-image Huffman trees?** They adapt to the image and need no
built-in tables. The tree costs a few hundred bytes, which is negligible for
photos and noticeable only for thumbnails.

**Why not arithmetic coding?** It would save a further 5 to 10%, but a
correct range coder is a project of its own.

**Where the remaining time goes.** After the transform was parallelized,
entropy coding and memory traffic dominate. The planes are held as float64,
which is simple but uses eight bytes per sample.

## Limitations

- The `.dct` format is specific to this project.
- Alpha is not stored. Transparent images are encoded with premultiplied
  color and decode as opaque.
- No progressive mode, no restart markers, no streaming: the whole image is
  held in memory.

## License

MIT. See [LICENSE](LICENSE).

## Author

Shashank S ([@203SHASHANK](https://github.com/203SHASHANK))
