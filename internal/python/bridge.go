// Package python runs the reference JPEG encoders (Pillow and OpenCV) as
// subprocesses, so the benchmark can compare this codec against them.
package python

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// scriptTimeout bounds one script run. Encoding takes milliseconds; nearly
// all of the budget is for interpreter and library start-up.
const scriptTimeout = 60 * time.Second

// ScriptsDirEnv names the environment variable that overrides where the
// benchmark scripts are looked up.
const ScriptsDirEnv = "IMAGECOMPRESSOR_SCRIPTS_DIR"

// Scripts shipped in the scripts directory.
const (
	ScriptPIL    = "compress_pil.py"
	ScriptOpenCV = "compress_opencv.py"
)

// Result is what one script run produced.
type Result struct {
	Method         string
	Language       string
	CompressedSize int
	EncodingTime   time.Duration
	DecodingTime   time.Duration

	// Decoded is the image after a JPEG encode/decode round trip. The
	// scripts hand it back losslessly so that quality metrics for every
	// codec are computed by the same Go code.
	Decoded image.Image
}

// Bridge locates the Python interpreter and the benchmark scripts.
type Bridge struct {
	interpreter string // empty when no interpreter was found
	scriptsDir  string
}

// NewBridge looks up the interpreter and scripts directory once.
func NewBridge() *Bridge {
	name := "python3"
	if runtime.GOOS == "windows" {
		name = "python"
	}
	interpreter, err := exec.LookPath(name)
	if err != nil {
		interpreter = ""
	}
	return &Bridge{interpreter: interpreter, scriptsDir: findScriptsDir()}
}

// IsAvailable reports whether a Python interpreter was found. Whether the
// libraries a script needs are installed only shows when it runs.
func (b *Bridge) IsAvailable() bool {
	return b.interpreter != ""
}

// Run compresses the image at inputPath with the named script at the given
// JPEG quality. Cancelling ctx kills the subprocess.
func (b *Bridge) Run(ctx context.Context, script, inputPath string, quality int) (*Result, error) {
	if !b.IsAvailable() {
		return nil, fmt.Errorf("python interpreter not found")
	}

	// Each run gets a private directory, so concurrent benchmarks never
	// write to the same files.
	workDir, err := os.MkdirTemp("", "imgcomp-py-")
	if err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	jpegPath := filepath.Join(workDir, "out.jpg")
	decodedPath := filepath.Join(workDir, "decoded.png")

	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, b.interpreter,
		filepath.Join(b.scriptsDir, script),
		"--input", inputPath,
		"--quality", strconv.Itoa(quality),
		"--jpeg", jpegPath,
		"--decoded", decodedPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", script, err, bytes.TrimSpace(stderr.Bytes()))
	}

	var report struct {
		Method         string  `json:"method"`
		Language       string  `json:"language"`
		CompressedSize int     `json:"compressed_size"`
		EncodingTimeMS float64 `json:"encoding_time_ms"`
		DecodingTimeMS float64 `json:"decoding_time_ms"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return nil, fmt.Errorf("%s: parse output: %w", script, err)
	}

	decoded, err := readPNG(decodedPath)
	if err != nil {
		return nil, fmt.Errorf("%s: read decoded image: %w", script, err)
	}

	return &Result{
		Method:         report.Method,
		Language:       report.Language,
		CompressedSize: report.CompressedSize,
		EncodingTime:   time.Duration(report.EncodingTimeMS * float64(time.Millisecond)),
		DecodingTime:   time.Duration(report.DecodingTimeMS * float64(time.Millisecond)),
		Decoded:        decoded,
	}, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// findScriptsDir returns the first existing candidate: the directory named
// by ScriptsDirEnv, the in-repo location relative to the working directory
// (go run, tests, the Docker image), then the same location next to the
// executable.
func findScriptsDir() string {
	relative := filepath.Join("internal", "python", "scripts")

	candidates := []string{os.Getenv(ScriptsDirEnv), relative}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), relative),
			filepath.Join(filepath.Dir(exe), "..", relative))
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return relative
}
