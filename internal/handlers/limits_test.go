package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func uploadRequest(t *testing.T, fileContents []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("image", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(fileContents); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func tinyPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A PNG a few dozen bytes long can claim to be 60000x60000 pixels. The
// upload must be refused from the header alone, before 14 GB is allocated.
func TestUploadRejectsDecompressionBomb(t *testing.T) {
	bomb := tinyPNG(t, 1, 1)
	// IHDR data starts at byte 16: width then height, big-endian. The chunk
	// checksum covers the type and data (bytes 12-28) and follows them.
	binary.BigEndian.PutUint32(bomb[16:], 60000)
	binary.BigEndian.PutUint32(bomb[20:], 60000)
	binary.BigEndian.PutUint32(bomb[29:], crc32.ChecksumIEEE(bomb[12:29]))

	rec := httptest.NewRecorder()
	UploadHandler(rec, uploadRequest(t, bomb))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body)
	}
}

func TestUploadRejectsOversizedFile(t *testing.T) {
	rec := httptest.NewRecorder()
	UploadHandler(rec, uploadRequest(t, make([]byte, maxUploadBytes+2<<20)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestUploadRejectsNonImage(t *testing.T) {
	rec := httptest.NewRecorder()
	UploadHandler(rec, uploadRequest(t, []byte("this is not an image")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// uploadImage stores an image through the real handler and returns its ID.
func uploadImage(t *testing.T, width, height int) string {
	t.Helper()
	rec := httptest.NewRecorder()
	UploadHandler(rec, uploadRequest(t, tinyPNG(t, width, height)))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body)
	}
	var meta struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	return meta.ID
}

func compress(t *testing.T, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/compress", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	CompressHandler(rec, req)
	return rec
}

func TestCompressRejectsUnknownSubsampling(t *testing.T) {
	id := uploadImage(t, 16, 16)
	rec := compress(t, `{"image_id":"`+id+`","quality":75,"chroma_subsampling":"4:1:1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCompressRejectsHugeBody(t *testing.T) {
	rec := compress(t, `{"image_id":"`+strings.Repeat("x", maxJSONBytes)+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Images smaller than the SSIM window must still compress.
func TestCompressTinyImage(t *testing.T) {
	id := uploadImage(t, 3, 2)
	rec := compress(t, `{"image_id":"`+id+`","quality":80}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body)
	}
}

func TestDownloadHeaders(t *testing.T) {
	id := uploadImage(t, 32, 32)
	rec := compress(t, `{"image_id":"`+id+`","quality":75}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("compress failed: %d %s", rec.Code, rec.Body)
	}
	var result struct {
		CompressedURL string `json:"compressed_image_url"`
		PreviewURL    string `json:"preview_url"`
		ExportID      string `json:"export_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	get := func(handler http.HandlerFunc, url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec
	}

	dct := get(DownloadHandler, result.CompressedURL)
	if got := dct.Header().Get("Content-Disposition"); got != `attachment; filename="compressed.dct"` {
		t.Errorf(".dct disposition = %q", got)
	}
	if !bytes.HasPrefix(dct.Body.Bytes(), []byte("DCT\x02")) {
		t.Error(".dct download does not start with the format magic")
	}

	preview := get(DownloadHandler, result.PreviewURL)
	if got := preview.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("preview content type = %q", got)
	}
	if got := preview.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
		t.Errorf("preview disposition = %q, want inline", got)
	}

	for format, wantType := range map[string]string{"png": "image/png", "jpeg": "image/jpeg", "dct": "application/octet-stream"} {
		rec := get(ExportHandler, "/api/export?id="+result.ExportID+"&format="+format)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != wantType {
			t.Errorf("export %s: status %d, type %q", format, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	if rec := get(ExportHandler, "/api/export?id="+result.ExportID+"&format=gif"); rec.Code != http.StatusBadRequest {
		t.Errorf("export gif: status %d, want 400", rec.Code)
	}
}
