package integration_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"imagecompressor-dct/internal/handlers"
)

// buildTestPNG creates a minimal in-memory PNG for upload tests.
func buildTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: 128,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// buildMultipartUpload wraps PNG bytes in a multipart/form-data body.
func buildMultipartUpload(pngData []byte) (*bytes.Buffer, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("image", "test.png")
	part.Write(pngData)
	writer.Close()
	return body, writer.FormDataContentType()
}

// ── Upload endpoint ────────────────────────────────────────────────────────

func TestUploadHandler_ValidPNG(t *testing.T) {
	pngData := buildTestPNG(64, 64)
	body, contentType := buildMultipartUpload(pngData)

	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	handlers.UploadHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["id"] == nil {
		t.Error("response missing 'id' field")
	}
	if response["width"].(float64) != 64 {
		t.Errorf("expected width=64, got %v", response["width"])
	}
	if response["height"].(float64) != 64 {
		t.Errorf("expected height=64, got %v", response["height"])
	}
}

func TestUploadHandler_WrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/upload", nil)
	rec := httptest.NewRecorder()

	handlers.UploadHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestUploadHandler_MissingFile(t *testing.T) {
	body := strings.NewReader("no file here")
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	rec := httptest.NewRecorder()

	handlers.UploadHandler(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected non-200 for missing file")
	}
}

// ── Compress endpoint ──────────────────────────────────────────────────────

func TestCompressHandler_FullFlow(t *testing.T) {
	// Step 1: upload.
	pngData := buildTestPNG(64, 64)
	body, contentType := buildMultipartUpload(pngData)
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	uploadReq.Header.Set("Content-Type", contentType)
	uploadRec := httptest.NewRecorder()
	handlers.UploadHandler(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	json.NewDecoder(uploadRec.Body).Decode(&uploadResp)
	imageID := uploadResp["id"].(string)

	// Step 2: compress.
	compressPayload, _ := json.Marshal(map[string]interface{}{
		"image_id":           imageID,
		"quality":            75,
		"chroma_subsampling": "4:2:0",
	})
	compressReq := httptest.NewRequest(http.MethodPost, "/api/compress", bytes.NewReader(compressPayload))
	compressReq.Header.Set("Content-Type", "application/json")
	compressRec := httptest.NewRecorder()
	handlers.CompressHandler(compressRec, compressReq)

	if compressRec.Code != http.StatusOK {
		t.Fatalf("compress failed: %d %s", compressRec.Code, compressRec.Body.String())
	}

	var compressResp map[string]interface{}
	json.NewDecoder(compressRec.Body).Decode(&compressResp)

	if compressResp["compression_ratio"] == nil {
		t.Error("response missing 'compression_ratio'")
	}
	if compressResp["psnr"] == nil {
		t.Error("response missing 'psnr'")
	}
	if compressResp["ssim"] == nil {
		t.Error("response missing 'ssim'")
	}

	ratio := compressResp["compression_ratio"].(float64)
	if ratio <= 0 {
		t.Errorf("compression_ratio should be > 0, got %f", ratio)
	}
}

func TestCompressHandler_InvalidQuality(t *testing.T) {
	compressPayload, _ := json.Marshal(map[string]interface{}{
		"image_id": "img_1",
		"quality":  0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/compress", bytes.NewReader(compressPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CompressHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for quality=0, got %d", rec.Code)
	}
}

func TestCompressHandler_UnknownImageID(t *testing.T) {
	compressPayload, _ := json.Marshal(map[string]interface{}{
		"image_id": "nonexistent_id",
		"quality":  75,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/compress", bytes.NewReader(compressPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.CompressHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown image ID, got %d", rec.Code)
	}
}

// ── Formats endpoint ───────────────────────────────────────────────────────

func TestFormatsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/formats", nil)
	rec := httptest.NewRecorder()

	handlers.FormatsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&response)

	if response["input_formats"] == nil {
		t.Error("response missing 'input_formats'")
	}
}

// ── Download endpoint ──────────────────────────────────────────────────────

func TestDownloadHandler_AfterCompress(t *testing.T) {
	// Upload an image.
	pngData := buildTestPNG(64, 64)
	body, contentType := buildMultipartUpload(pngData)
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	uploadReq.Header.Set("Content-Type", contentType)
	uploadRec := httptest.NewRecorder()
	handlers.UploadHandler(uploadRec, uploadReq)

	var uploadResp map[string]interface{}
	json.NewDecoder(uploadRec.Body).Decode(&uploadResp)
	imageID := uploadResp["id"].(string)

	// Compress it.
	compressPayload, _ := json.Marshal(map[string]interface{}{
		"image_id": imageID,
		"quality":  75,
	})
	compressReq := httptest.NewRequest(http.MethodPost, "/api/compress", bytes.NewReader(compressPayload))
	compressReq.Header.Set("Content-Type", "application/json")
	compressRec := httptest.NewRecorder()
	handlers.CompressHandler(compressRec, compressReq)

	if compressRec.Code != http.StatusOK {
		t.Fatalf("compress failed: %d %s", compressRec.Code, compressRec.Body.String())
	}

	var compressResp map[string]interface{}
	json.NewDecoder(compressRec.Body).Decode(&compressResp)

	downloadURL, ok := compressResp["compressed_image_url"].(string)
	if !ok || downloadURL == "" {
		t.Fatal("compress response missing 'compressed_image_url'")
	}

	// Download the compressed file.
	downloadReq := httptest.NewRequest(http.MethodGet, downloadURL, nil)
	downloadRec := httptest.NewRecorder()
	handlers.DownloadHandler(downloadRec, downloadReq)

	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download failed: %d %s", downloadRec.Code, downloadRec.Body.String())
	}
	if downloadRec.Body.Len() == 0 {
		t.Error("download response body is empty")
	}
}

func TestDownloadHandler_UnknownID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/download/nonexistent_dl", nil)
	rec := httptest.NewRecorder()
	handlers.DownloadHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ── Benchmark endpoint ─────────────────────────────────────────────────────

func TestBenchmarkHandler_FullFlow(t *testing.T) {
	// Upload first.
	pngData := buildTestPNG(64, 64)
	body, contentType := buildMultipartUpload(pngData)
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	uploadReq.Header.Set("Content-Type", contentType)
	uploadRec := httptest.NewRecorder()
	handlers.UploadHandler(uploadRec, uploadReq)

	var uploadResp map[string]interface{}
	json.NewDecoder(uploadRec.Body).Decode(&uploadResp)
	imageID := uploadResp["id"].(string)

	// Run benchmark.
	benchPayload, _ := json.Marshal(map[string]interface{}{
		"image_id": imageID,
		"quality":  75,
	})
	benchReq := httptest.NewRequest(http.MethodPost, "/api/benchmark", bytes.NewReader(benchPayload))
	benchReq.Header.Set("Content-Type", "application/json")
	benchRec := httptest.NewRecorder()

	handlers.BenchmarkHandler(benchRec, benchReq)

	if benchRec.Code != http.StatusOK {
		t.Fatalf("benchmark failed: %d %s", benchRec.Code, benchRec.Body.String())
	}

	var benchResp map[string]interface{}
	json.NewDecoder(benchRec.Body).Decode(&benchResp)

	results, ok := benchResp["results"].([]interface{})
	if !ok || len(results) == 0 {
		t.Error("expected at least one benchmark result")
	}
}
