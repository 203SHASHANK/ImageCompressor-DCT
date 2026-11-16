#!/usr/bin/env python3
"""Encode an image as JPEG with OpenCV and report size and timings as JSON.

The decoded result is written out as a lossless PNG so that the caller can
compute quality metrics with the same code for every codec.
"""

import argparse
import json
import os
import time

import cv2


def run(input_path: str, quality: int, jpeg_path: str, decoded_path: str) -> dict:
    original = cv2.imread(input_path, cv2.IMREAD_COLOR)
    if original is None:
        raise SystemExit(f"could not read image: {input_path}")

    start = time.perf_counter()
    ok = cv2.imwrite(jpeg_path, original, [cv2.IMWRITE_JPEG_QUALITY, quality])
    encoding_ms = (time.perf_counter() - start) * 1000
    if not ok:
        raise SystemExit(f"could not write {jpeg_path}")

    start = time.perf_counter()
    decoded = cv2.imread(jpeg_path, cv2.IMREAD_COLOR)
    decoding_ms = (time.perf_counter() - start) * 1000

    cv2.imwrite(decoded_path, decoded, [cv2.IMWRITE_PNG_COMPRESSION, 1])

    return {
        "method": "Python OpenCV",
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
