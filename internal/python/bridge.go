// Package python provides a bridge for executing Python benchmark scripts
// as subprocesses and parsing their JSON output into BenchmarkResult values.
package python

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"imagecompressor-dct/internal/models"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const scriptTimeout = 60 * time.Second

// Bridge executes Python compression scripts and parses their results.
type Bridge struct {
	pythonExecutable string
	scriptsDirectory string
}

// NewBridge creates a Bridge, auto-detecting the Python executable.
func NewBridge() *Bridge {
	return &Bridge{
		pythonExecutable: detectPython(),
		scriptsDirectory: resolveScriptsDirectory(),
	}
}

// IsAvailable returns true if a usable Python interpreter was found.
func (bridge *Bridge) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, bridge.pythonExecutable, "--version").Run()
	return err == nil
}

// RunPILBenchmark compresses imagePath with Pillow and returns metrics.
func (bridge *Bridge) RunPILBenchmark(imagePath string, quality int) (models.BenchmarkResult, error) {
	return bridge.runScript("compress_pil.py", imagePath, quality)
}

// RunOpenCVBenchmark compresses imagePath with OpenCV and returns metrics.
func (bridge *Bridge) RunOpenCVBenchmark(imagePath string, quality int) (models.BenchmarkResult, error) {
	return bridge.runScript("compress_opencv.py", imagePath, quality)
}

// runScript executes a named Python script with the given arguments and
// parses its stdout as a JSON BenchmarkResult.
func (bridge *Bridge) runScript(scriptName, imagePath string, quality int) (models.BenchmarkResult, error) {
	scriptPath := filepath.Join(bridge.scriptsDirectory, scriptName)

	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		bridge.pythonExecutable,
		scriptPath,
		"--input", imagePath,
		"--quality", fmt.Sprintf("%d", quality),
		"--output-json",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("script %q failed: %w\nstderr: %s", scriptName, err, stderr.String())
	}
	outputBytes := stdout.Bytes()

	var rawResult struct {
		Method           string  `json:"method"`
		Language         string  `json:"language"`
		CompressedSize   int     `json:"compressed_size"`
		CompressionRatio float64 `json:"compression_ratio"`
		EncodingTimeMS   float64 `json:"encoding_time_ms"`
		DecodingTimeMS   float64 `json:"decoding_time_ms"`
		PSNR             float64 `json:"psnr"`
		SSIM             float64 `json:"ssim"`
	}

	if err := json.Unmarshal(outputBytes, &rawResult); err != nil {
		return models.BenchmarkResult{}, fmt.Errorf("failed to parse output from %q: %w", scriptName, err)
	}

	return models.BenchmarkResult{
		Method:           rawResult.Method,
		Language:         rawResult.Language,
		CompressedSize:   rawResult.CompressedSize,
		CompressionRatio: rawResult.CompressionRatio,
		EncodingTime:     time.Duration(rawResult.EncodingTimeMS * float64(time.Millisecond)),
		DecodingTime:     time.Duration(rawResult.DecodingTimeMS * float64(time.Millisecond)),
		PSNR:             rawResult.PSNR,
		SSIM:             rawResult.SSIM,
	}, nil
}

// detectPython returns "python3" on Unix or "python" on Windows.
func detectPython() string {
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}

// resolveScriptsDirectory returns the path to the Python scripts directory
// relative to the project root (works when running from project root or bin/).
func resolveScriptsDirectory() string {
	return filepath.Join("internal", "python", "scripts")
}
