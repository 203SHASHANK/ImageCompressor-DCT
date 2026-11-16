// Command dctpress is the command-line front end of the codec.
//
//	dctpress encode [-q 75] [-s 4:2:0] <image> <out.dct>
//	dctpress decode <in.dct> <out.png>
//	dctpress bench  [-q 75] [-runs 5] <image>
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"sort"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/models"
	"imagecompressor-dct/internal/services"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "encode":
		err = encode(os.Args[2:])
	case "decode":
		err = decode(os.Args[2:])
	case "bench":
		err = bench(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dctpress:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  dctpress encode [-q quality] [-s 4:4:4|4:2:2|4:2:0] <image> <out.dct>
  dctpress decode <in.dct> <out.png>
  dctpress bench  [-q quality] [-runs n] <image>
`)
	os.Exit(2)
}

func encode(args []string) error {
	fs := flag.NewFlagSet("encode", flag.ExitOnError)
	quality := fs.Int("q", 75, "quality, 1-100")
	mode := fs.String("s", "4:2:0", "chroma subsampling")
	fs.Parse(args)
	if fs.NArg() != 2 {
		usage()
	}

	subsampling, ok := map[string]dct.ChromaSubsampling{
		"4:4:4": dct.Subsampling444, "4:2:2": dct.Subsampling422, "4:2:0": dct.Subsampling420,
	}[*mode]
	if !ok {
		return fmt.Errorf("unknown subsampling %q", *mode)
	}

	img, err := loadImage(fs.Arg(0))
	if err != nil {
		return err
	}
	encoder, err := dct.NewEncoder(dct.CompressionOptions{Quality: *quality, ChromaSubsampling: subsampling})
	if err != nil {
		return err
	}
	result, err := encoder.Encode(img)
	if err != nil {
		return err
	}
	if err := os.WriteFile(fs.Arg(1), result.Data, 0o644); err != nil {
		return err
	}
	fmt.Printf("%dx%d: %d bytes, %.1fx smaller than raw RGB\n",
		result.Width, result.Height, result.CompressedSize, result.CompressionRatio)
	return nil
}

func decode(args []string) error {
	if len(args) != 2 {
		usage()
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	img, err := dct.NewDecoder().Decode(data)
	if err != nil {
		return err
	}
	out, err := os.Create(args[1])
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// bench runs every available codec on one image several times and prints
// the median timings, so a single slow run does not skew the table.
func bench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	quality := fs.Int("q", 75, "quality, 1-100")
	runs := fs.Int("runs", 5, "number of runs; the median time is reported")
	fs.Parse(args)
	if fs.NArg() != 1 || *runs < 1 {
		usage()
	}

	img, err := loadImage(fs.Arg(0))
	if err != nil {
		return err
	}

	service := services.NewBenchmarkService()
	var rows []models.BenchmarkResult
	encodeTimes := map[string][]time.Duration{}
	decodeTimes := map[string][]time.Duration{}
	for i := 0; i < *runs; i++ {
		results := service.RunAll(context.Background(), img, *quality)
		if i == 0 {
			rows = results
		}
		for _, r := range results {
			encodeTimes[r.Method] = append(encodeTimes[r.Method], r.EncodingTime)
			decodeTimes[r.Method] = append(decodeTimes[r.Method], r.DecodingTime)
		}
	}

	b := img.Bounds()
	fmt.Printf("%s, %dx%d, quality %d, median of %d runs\n\n", fs.Arg(0), b.Dx(), b.Dy(), *quality, *runs)
	fmt.Printf("%-16s %10s %8s %9s %8s %10s %10s\n", "codec", "bytes", "ratio", "PSNR dB", "SSIM", "encode ms", "decode ms")
	for _, r := range rows {
		fmt.Printf("%-16s %10d %7.1fx %9.2f %8.4f %10.1f %10.1f\n",
			r.Method, r.CompressedSize, r.CompressionRatio, r.PSNR, r.SSIM,
			milliseconds(median(encodeTimes[r.Method])), milliseconds(median(decodeTimes[r.Method])))
	}
	return nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

func median(d []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), d...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func milliseconds(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
