#!/usr/bin/env python3
"""Encode an image as JPEG with Pillow and report size and timings as JSON.

The decoded result is written out as a lossless PNG so that the caller can
compute quality metrics with the same code for every codec.
"""

import argparse
import json
import os
import time

from PIL import Image


def run(input_path: str, quality: int, jpeg_path: str, decoded_path: str) -> dict:
    original = Image.open(input_path).convert("RGB")

    start = time.perf_counter()
    original.save(jpeg_path, "JPEG", quality=quality, optimize=True)
    encoding_ms = (time.perf_counter() - start) * 1000

    start = time.perf_counter()
    decoded = Image.open(jpeg_path).convert("RGB")
    decoded.load()  # Image.open is lazy; force the actual decode
    decoding_ms = (time.perf_counter() - start) * 1000

    decoded.save(decoded_path, "PNG", compress_level=1)

    return {
        "method": "Python PIL",
        "language": "Python",
        "compressed_size": os.path.getsize(jpeg_path),
        "encoding_time_ms": encoding_ms,
        "decoding_time_ms": decoding_ms,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, help="image to compress")
    parser.add_argument("--quality", type=int, required=True, help="JPEG quality, 1-100")
    parser.add_argument("--jpeg", required=True, help="where to write the JPEG")
    parser.add_argument("--decoded", required=True, help="where to write the decoded PNG")
    args = parser.parse_args()

    print(json.dumps(run(args.input, args.quality, args.jpeg, args.decoded)))
