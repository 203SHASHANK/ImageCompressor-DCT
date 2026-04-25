// Package handlers contains HTTP request handlers for the compression API.
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/models"
	"imagecompressor-dct/internal/services"
	"net/http"
	"strings"
	"sync"
)

const maxUploadBytes = 10 * 1024 * 1024 // 10 MB

// ── In-memory image store ──────────────────────────────────────────────────

// imageEntry pairs a decoded image with the byte size of its original uploaded file.
type imageEntry struct {
	img      image.Image
	fileSize int // actual uploaded file size in bytes
}

var (
	imageStore   = make(map[string]imageEntry)
	imageStoreMu sync.RWMutex
	imageIDSeq   int
)

func storeImage(img image.Image, fileSize int) string {
	imageStoreMu.Lock()
	defer imageStoreMu.Unlock()
	imageIDSeq++
	id := fmt.Sprintf("img_%d", imageIDSeq)
	imageStore[id] = imageEntry{img: img, fileSize: fileSize}
	return id
}

func retrieveImageEntry(id string) (imageEntry, bool) {
	imageStoreMu.RLock()
	defer imageStoreMu.RUnlock()
	entry, ok := imageStore[id]
	return entry, ok
}

// ── In-memory download store ───────────────────────────────────────────────

type downloadEntry struct {
	data        []byte
	contentType string
}

var (
	downloadStore   = make(map[string]downloadEntry)
	downloadStoreMu sync.RWMutex
	downloadIDSeq   int
)

func storeDownload(data []byte, contentType string) string {
	downloadStoreMu.Lock()
	defer downloadStoreMu.Unlock()
	downloadIDSeq++
	id := fmt.Sprintf("dl_%d", downloadIDSeq)
	downloadStore[id] = downloadEntry{data: data, contentType: contentType}
	return id
}

func retrieveDownload(id string) (downloadEntry, bool) {
	downloadStoreMu.RLock()
	defer downloadStoreMu.RUnlock()
	entry, ok := downloadStore[id]
	return entry, ok
}

// ── In-memory decoded image store ─────────────────────────────────────────

type decodedEntry struct {
	img     image.Image
	quality int
}

var (
	decodedStore   = make(map[string]decodedEntry)
	decodedStoreMu sync.RWMutex
)

func storeDecodedImage(id string, img image.Image, quality int) {
	decodedStoreMu.Lock()
	defer decodedStoreMu.Unlock()
	decodedStore[id] = decodedEntry{img: img, quality: quality}
}

func retrieveDecodedImage(id string) (decodedEntry, bool) {
	decodedStoreMu.RLock()
	defer decodedStoreMu.RUnlock()
	entry, ok := decodedStore[id]
	return entry, ok
}

// ── Helpers ────────────────────────────────────────────────────────────────

func writeJSON(writer http.ResponseWriter, statusCode int, payload interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, statusCode int, message string) {
	writeJSON(writer, statusCode, map[string]string{"error": message})
}

// ── UploadHandler ──────────────────────────────────────────────────────────

// UploadHandler handles POST /api/upload — accepts a multipart image file.
func UploadHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := request.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(writer, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := request.FormFile("image")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "missing 'image' field in form: "+err.Error())
		return
	}
	defer file.Close()

	if header.Size > maxUploadBytes {
		writeError(writer, http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large: max %d bytes", maxUploadBytes))
		return
	}

	img, format, err := image.Decode(file)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "unsupported or corrupt image: "+err.Error())
		return
	}

	imageID := storeImage(img, int(header.Size))
	bounds := img.Bounds()

	writeJSON(writer, http.StatusOK, models.ImageMetadata{
		ID:        imageID,
		Format:    format,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		SizeBytes: int(header.Size),
		ColorMode: "RGB",
	})
}

// ── CompressHandler ────────────────────────────────────────────────────────

// CompressHandler handles POST /api/compress — compresses a previously uploaded image.
func CompressHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var requestBody struct {
		ImageID           string `json:"image_id"`
		Quality           int    `json:"quality"`
		ChromaSubsampling string `json:"chroma_subsampling"`
	}

	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if requestBody.Quality < 1 || requestBody.Quality > 100 {
		writeError(writer, http.StatusBadRequest, "quality must be between 1 and 100")
		return
	}

	entry, ok := retrieveImageEntry(requestBody.ImageID)
	if !ok {
		writeError(writer, http.StatusNotFound, fmt.Sprintf("image %q not found", requestBody.ImageID))
		return
	}

	subsampling := parseChromaSubsampling(requestBody.ChromaSubsampling)

	compressionService := services.NewCompressionService()
	response, err := compressionService.Compress(services.CompressRequest{
		Image:             entry.img,
		Quality:           requestBody.Quality,
		ChromaSubsampling: subsampling,
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "compression failed: "+err.Error())
		return
	}

	downloadID := storeDownload(response.Result.Data, "application/octet-stream")

	// Store decoded image + quality for export endpoint.
	storeDecodedImage(downloadID, response.DecodedImage, requestBody.Quality)

	// Encode the decompressed image as JPEG so the browser can display it.
	var previewBuf bytes.Buffer
	previewID := ""
	if err := jpeg.Encode(&previewBuf, response.DecodedImage, &jpeg.Options{Quality: 90}); err == nil {
		previewID = storeDownload(previewBuf.Bytes(), "image/jpeg")
	}

	fileRatio := 0.0
	if response.Result.CompressedSize > 0 {
		fileRatio = float64(entry.fileSize) / float64(response.Result.CompressedSize)
	}

	resp := map[string]interface{}{
		"compressed_image_url": fmt.Sprintf("/api/download/%s", downloadID),
		"export_id":            downloadID,
		"original_size":        response.Result.OriginalSize,
		"compressed_size":      response.Result.CompressedSize,
		"compression_ratio":    response.Result.CompressionRatio,
		"file_original_size":   entry.fileSize,
		"file_ratio":           fileRatio,
		"encoding_time_ms":     response.EncodingTime.Milliseconds(),
		"psnr":                 response.PSNR,
		"ssim":                 response.SSIM,
		"width":                response.Result.Width,
		"height":               response.Result.Height,
	}
	if previewID != "" {
		resp["preview_url"] = fmt.Sprintf("/api/download/%s", previewID)
	}
	writeJSON(writer, http.StatusOK, resp)
}

// ── BenchmarkHandler ───────────────────────────────────────────────────────

// BenchmarkHandler handles POST /api/benchmark — runs all compression methods.
func BenchmarkHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var requestBody struct {
		ImageID string `json:"image_id"`
		Quality int    `json:"quality"`
	}

	if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	entry, ok := retrieveImageEntry(requestBody.ImageID)
	if !ok {
		writeError(writer, http.StatusNotFound, fmt.Sprintf("image %q not found", requestBody.ImageID))
		return
	}

	quality := requestBody.Quality
	if quality < 1 || quality > 100 {
		quality = 75
	}

	benchmarkService := services.NewBenchmarkService()
	results := benchmarkService.RunAll(entry.img, quality)

	writeJSON(writer, http.StatusOK, map[string]interface{}{
		"quality": quality,
		"results": results,
	})
}

// ── DownloadHandler ────────────────────────────────────────────────────────

// DownloadHandler handles GET /api/download/{id} — serves compressed file bytes.
func DownloadHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	fileID := strings.TrimPrefix(request.URL.Path, "/api/download/")
	if fileID == "" {
		writeError(writer, http.StatusBadRequest, "missing file ID in path")
		return
	}

	entry, ok := retrieveDownload(fileID)
	if !ok {
		writeError(writer, http.StatusNotFound, fmt.Sprintf("download %q not found", fileID))
		return
	}

	writer.Header().Set("Content-Type", entry.contentType)
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.dct"`, fileID))
	writer.WriteHeader(http.StatusOK)
	writer.Write(entry.data)
}

// ── ExportHandler ─────────────────────────────────────────────────────────

// ExportHandler handles GET /api/export?id=<id>&format=<jpeg|png|dct>
func ExportHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := request.URL.Query().Get("id")
	format := request.URL.Query().Get("format")
	if id == "" {
		writeError(writer, http.StatusBadRequest, "missing id parameter")
		return
	}
	switch format {
	case "dct":
		entry, ok := retrieveDownload(id)
		if !ok {
			writeError(writer, http.StatusNotFound, "export not found")
			return
		}
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="compressed_%s.dct"`, id))
		writer.Write(entry.data)
	case "jpeg", "png", "":
		de, ok := retrieveDecodedImage(id)
		if !ok {
			writeError(writer, http.StatusNotFound, "decoded image not found")
			return
		}
		var buf bytes.Buffer
		if format == "png" {
			if err := png.Encode(&buf, de.img); err != nil {
				writeError(writer, http.StatusInternalServerError, err.Error())
				return
			}
			writer.Header().Set("Content-Type", "image/png")
			writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="compressed_%s.png"`, id))
		} else {
			if err := jpeg.Encode(&buf, de.img, &jpeg.Options{Quality: de.quality}); err != nil {
				writeError(writer, http.StatusInternalServerError, err.Error())
				return
			}
			writer.Header().Set("Content-Type", "image/jpeg")
			writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="compressed_%s.jpg"`, id))
		}
		writer.Write(buf.Bytes())
	default:
		writeError(writer, http.StatusBadRequest, "unsupported format: use jpeg, png, or dct")
	}
}

// ── FormatsHandler ─────────────────────────────────────────────────────────

// FormatsHandler handles GET /api/formats — returns supported input formats.
func FormatsHandler(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]interface{}{
		"input_formats":  []string{"png", "jpeg", "webp", "bmp", "tiff"},
		"output_formats": []string{"dct", "jpeg", "png"},
	})
}

// ── Helpers ────────────────────────────────────────────────────────────────

func parseChromaSubsampling(value string) dct.ChromaSubsampling {
	switch value {
	case "4:2:2":
		return dct.Subsampling422
	case "4:4:4":
		return dct.Subsampling444
	default:
		return dct.Subsampling420
	}
}
