#!/usr/bin/env python3
"""Compress an image with Pillow (PIL) and output metrics as JSON."""

import argparse
import json
import os
import sys
import time


def compress_with_pil(input_path: str, quality: int) -> dict:
    try:
        import numpy as np
        from PIL import Image
        from skimage.metrics import peak_signal_noise_ratio, structural_similarity
    except ImportError as e:
        print(json.dumps({"error": f"Missing dependency: {e}"}), file=sys.stderr)
        sys.exit(1)

    output_path = "/tmp/benchmark_pil_output.jpg"

    original = Image.open(input_path).convert("RGB")
    original_np = np.array(original)

    encode_start = time.perf_counter()
    original.save(output_path, "JPEG", quality=quality, optimize=True)
    encoding_time_ms = (time.perf_counter() - encode_start) * 1000

    decode_start = time.perf_counter()
    compressed = Image.open(output_path).convert("RGB")
    decoding_time_ms = (time.perf_counter() - decode_start) * 1000

    compressed_np = np.array(compressed)

    psnr = peak_signal_noise_ratio(original_np, compressed_np, data_range=255)
    ssim = structural_similarity(original_np, compressed_np, channel_axis=2, data_range=255)

    width, height = original.size
    raw_size = width * height * 3  # uncompressed RGB bytes, same baseline as Go encoder
    compressed_size = os.path.getsize(output_path)

    return {
        "method": "Python PIL",
        "language": "Python",
        "compressed_size": compressed_size,
        "compression_ratio": raw_size / compressed_size if compressed_size > 0 else 0,
        "encoding_time_ms": encoding_time_ms,
        "decoding_time_ms": decoding_time_ms,
        "psnr": float(psnr),
        "ssim": float(ssim),
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="PIL JPEG benchmark")
    parser.add_argument("--input", required=True, help="Path to input image")
    parser.add_argument("--quality", type=int, required=True, help="JPEG quality (1-100)")
    parser.add_argument("--output-json", action="store_true", help="Output results as JSON")
    args = parser.parse_args()

    result = compress_with_pil(args.input, args.quality)

    if args.output_json:
        print(json.dumps(result))
    else:
        print(f"Method:  {result['method']}")
        print(f"Size:    {result['compressed_size']} bytes")
        print(f"Ratio:   {result['compression_ratio']:.2f}x")
        print(f"PSNR:    {result['psnr']:.2f} dB")
        print(f"SSIM:    {result['ssim']:.4f}")
        print(f"Encode:  {result['encoding_time_ms']:.1f} ms")
