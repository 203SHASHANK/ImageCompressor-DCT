#!/usr/bin/env python3
"""Compress an image with OpenCV and output metrics as JSON."""

import argparse
import json
import os
import sys
import time


def compress_with_opencv(input_path: str, quality: int) -> dict:
    try:
        import cv2
        import numpy as np
        from skimage.metrics import peak_signal_noise_ratio, structural_similarity
    except ImportError as e:
        print(json.dumps({"error": f"Missing dependency: {e}"}), file=sys.stderr)
        sys.exit(1)

    output_path = "/tmp/benchmark_opencv_output.jpg"

    original_bgr = cv2.imread(input_path)
    if original_bgr is None:
        raise ValueError(f"Could not read image: {input_path}")
    original_rgb = cv2.cvtColor(original_bgr, cv2.COLOR_BGR2RGB)

    encode_start = time.perf_counter()
    cv2.imwrite(output_path, original_bgr, [cv2.IMWRITE_JPEG_QUALITY, quality])
    encoding_time_ms = (time.perf_counter() - encode_start) * 1000

    decode_start = time.perf_counter()
    compressed_bgr = cv2.imread(output_path)
    decoding_time_ms = (time.perf_counter() - decode_start) * 1000

    compressed_rgb = cv2.cvtColor(compressed_bgr, cv2.COLOR_BGR2RGB)

    psnr = peak_signal_noise_ratio(original_rgb, compressed_rgb, data_range=255)
    ssim = structural_similarity(original_rgb, compressed_rgb, channel_axis=2, data_range=255)

    h, w = original_rgb.shape[:2]
    raw_size = w * h * 3  # uncompressed RGB bytes, same baseline as Go encoder
    compressed_size = os.path.getsize(output_path)

    return {
        "method": "Python OpenCV",
        "language": "Python",
        "compressed_size": compressed_size,
        "compression_ratio": raw_size / compressed_size if compressed_size > 0 else 0,
        "encoding_time_ms": encoding_time_ms,
        "decoding_time_ms": decoding_time_ms,
        "psnr": float(psnr),
        "ssim": float(ssim),
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="OpenCV JPEG benchmark")
    parser.add_argument("--input", required=True, help="Path to input image")
    parser.add_argument("--quality", type=int, required=True, help="JPEG quality (1-100)")
    parser.add_argument("--output-json", action="store_true", help="Output results as JSON")
    args = parser.parse_args()

    result = compress_with_opencv(args.input, args.quality)

    if args.output_json:
        print(json.dumps(result))
    else:
        print(f"Method:  {result['method']}")
        print(f"Size:    {result['compressed_size']} bytes")
        print(f"Ratio:   {result['compression_ratio']:.2f}x")
        print(f"PSNR:    {result['psnr']:.2f} dB")
        print(f"SSIM:    {result['ssim']:.4f}")
        print(f"Encode:  {result['encoding_time_ms']:.1f} ms")
