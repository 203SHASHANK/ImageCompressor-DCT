// Package handlers implements the HTTP API: upload an image, compress it,
// benchmark it against other encoders, and download the results.
package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	// Registered for image.Decode. JPEG and PNG come with the imports above.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"imagecompressor-dct/internal/core/dct"
	"imagecompressor-dct/internal/models"
	"imagecompressor-dct/internal/services"
)

const (
	// maxUploadBytes limits the size of an uploaded file.
	maxUploadBytes = 10 << 20

	// maxImagePixels limits the decoded size. Compressed formats can declare
	// enormous dimensions in a tiny file, so the byte limit alone does not
	// bound memory use: 16 megapixels is 64 MB as RGBA.
	maxImagePixels = 16 << 20

	// maxJSONBytes limits the body of the JSON endpoints.
	maxJSONBytes = 1 << 20

	// previewQuality is the JPEG quality of the in-browser preview. It is
	// high so the preview shows this codec's artifacts, not JPEG's.
	previewQuality = 95

	entryLifetime = 30 * time.Minute
)

// uploadedImage is a decoded upload waiting to be compressed.
type uploadedImage struct {
	img      image.Image
	fileSize int // size of the uploaded file in bytes
}

// download is a blob the browser can fetch by ID.
type download struct {
	data        []byte
	contentType string
	filename    string
	inline      bool // display in the page instead of saving
}

// decodedImage is a compression result kept for the export endpoint.
type decodedImage struct {
	img     image.Image
	quality int
}

// Every compression produces two downloads (the .dct file and a preview),
// hence the larger capacity.
var (
	uploads   = newStore[uploadedImage](8, entryLifetime)
	downloads = newStore[download](32, entryLifetime)
	decoded   = newStore[decodedImage](16, entryLifetime)
)

// UploadHandler handles POST /api/upload: a multipart form with the image
// in the "image" field. It responds with the image's ID and metadata.
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// MaxBytesReader is what actually enforces the limit; the argument to
	// ParseMultipartForm only decides how much is buffered in memory before
	// spilling to disk. The extra megabyte covers the multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("file too large: max %d bytes", maxUploadBytes))
			return
		}
		writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, fileHeader, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'image' field in form: "+err.Error())
		return
	}
	defer file.Close()

	if fileHeader.Size > maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file too large: max %d bytes", maxUploadBytes))
		return
	}

	// Read only the header first to learn the dimensions, and refuse
	// oversized images before allocating memory for their pixels.
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported or corrupt image: "+err.Error())
		return
	}
	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxImagePixels {
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("image too large: %dx%d exceeds %d pixels", config.Width, config.Height, maxImagePixels))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read upload")
		return
	}

	img, format, err := image.Decode(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported or corrupt image: "+err.Error())
		return
	}

	id := newID("img_")
	uploads.put(id, uploadedImage{img: img, fileSize: int(fileHeader.Size)})

	writeJSON(w, http.StatusOK, models.ImageMetadata{
		ID:        id,
		Format:    format,
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
		SizeBytes: int(fileHeader.Size),
		ColorMode: "RGB",
	})
}

// CompressHandler handles POST /api/compress: compress a previously
// uploaded image and return sizes, timings, quality metrics and the URLs of
// the result.
func CompressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		ImageID           string `json:"image_id"`
		Quality           int    `json:"quality"`
		ChromaSubsampling string `json:"chroma_subsampling"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Quality < 1 || body.Quality > 100 {
		writeError(w, http.StatusBadRequest, "quality must be between 1 and 100")
		return
	}
	subsampling, ok := parseChromaSubsampling(body.ChromaSubsampling)
	if !ok {
		writeError(w, http.StatusBadRequest, `chroma_subsampling must be "4:4:4", "4:2:2" or "4:2:0"`)
		return
	}
	upload, ok := uploads.get(body.ImageID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("image %q not found", body.ImageID))
		return
	}

	response, err := services.NewCompressionService().Compress(services.CompressRequest{
		Image:             upload.img,
		Quality:           body.Quality,
		ChromaSubsampling: subsampling,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compression failed: "+err.Error())
		return
	}

	resultID := newID("dl_")
	downloads.put(resultID, download{
		data:        response.Result.Data,
		contentType: "application/octet-stream",
		filename:    "compressed.dct",
	})
	decoded.put(resultID, decodedImage{img: response.DecodedImage, quality: body.Quality})

	payload := map[string]any{
		"compressed_image_url": "/api/download/" + resultID,
		"export_id":            resultID,
		"original_size":        response.Result.OriginalSize,
		"compressed_size":      response.Result.CompressedSize,
		"compression_ratio":    response.Result.CompressionRatio,
		"file_original_size":   upload.fileSize,
		"file_ratio":           float64(upload.fileSize) / float64(response.Result.CompressedSize),
		"encoding_time_ms":     response.EncodingTime.Milliseconds(),
		"decoding_time_ms":     response.DecodingTime.Milliseconds(),
		"psnr":                 response.PSNR,
		"ssim":                 response.SSIM,
		"width":                response.Result.Width,
		"height":               response.Result.Height,
	}

	// Browsers cannot display .dct, so the decoded image is also offered as
	// a JPEG. A failure here only costs the preview.
	var preview bytes.Buffer
	if err := jpeg.Encode(&preview, response.DecodedImage, &jpeg.Options{Quality: previewQuality}); err == nil {
		previewID := newID("pv_")
		downloads.put(previewID, download{
			data:        preview.Bytes(),
			contentType: "image/jpeg",
			filename:    "preview.jpg",
			inline:      true,
		})
		payload["preview_url"] = "/api/download/" + previewID
	}

	writeJSON(w, http.StatusOK, payload)
}

// BenchmarkHandler handles POST /api/benchmark: run the uploaded image
// through this codec and the reference encoders at the same quality.
func BenchmarkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		ImageID string `json:"image_id"`
		Quality int    `json:"quality"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	upload, ok := uploads.get(body.ImageID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("image %q not found", body.ImageID))
		return
	}

	quality := body.Quality
	if quality < 1 || quality > 100 {
		quality = 75
	}

	// The request context is cancelled when the client disconnects, which
	// stops any Python subprocess still running.
	results := services.NewBenchmarkService().RunAll(r.Context(), upload.img, quality)

	writeJSON(w, http.StatusOK, map[string]any{
		"quality": quality,
		"results": results,
	})
}

// DownloadHandler handles GET /api/download/{id}: serve a stored result.
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing file ID in path")
		return
	}
	entry, ok := downloads.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("download %q not found", id))
		return
	}
	writeFile(w, entry)
}

// ExportHandler handles GET /api/export?id=<id>&format=<jpeg|png|dct>: the
// compression result as a .dct file, or decoded and re-saved as JPEG or PNG.
func ExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id parameter")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jpeg"
	}

	if format == "dct" {
		entry, ok := downloads.get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "export not found")
			return
		}
		writeFile(w, entry)
		return
	}

	if format != "jpeg" && format != "png" {
		writeError(w, http.StatusBadRequest, "unsupported format: use jpeg, png, or dct")
		return
	}
	result, ok := decoded.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "decoded image not found")
		return
	}

	// Encode into a buffer first so an encoding error can still be reported
	// with a proper status code.
	var buf bytes.Buffer
	out := download{data: nil, contentType: "image/jpeg", filename: "compressed.jpg"}
	var err error
	if format == "png" {
		out.contentType, out.filename = "image/png", "compressed.png"
		err = png.Encode(&buf, result.img)
	} else {
		err = jpeg.Encode(&buf, result.img, &jpeg.Options{Quality: result.quality})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export failed: "+err.Error())
		return
	}
	out.data = buf.Bytes()
	writeFile(w, out)
}

// FormatsHandler handles GET /api/formats: the formats accepted for upload
// and offered for export.
func FormatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"input_formats":  []string{"png", "jpeg", "webp", "bmp", "tiff"},
		"output_formats": []string{"dct", "jpeg", "png"},
	})
}

// parseChromaSubsampling maps the API's string form to the codec's constant.
// An empty value selects 4:2:0, the usual default for photos.
func parseChromaSubsampling(value string) (dct.ChromaSubsampling, bool) {
	switch value {
	case "", "4:2:0":
		return dct.Subsampling420, true
	case "4:2:2":
		return dct.Subsampling422, true
	case "4:4:4":
		return dct.Subsampling444, true
	}
	return 0, false
}

// readJSON decodes a size-limited JSON request body into dst. On failure it
// writes the error response and returns false.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent; all that is left is to log it.
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeFile sends a stored blob with headers telling the browser whether to
// display it or save it, and under which name.
func writeFile(w http.ResponseWriter, d download) {
	disposition := "attachment"
	if d.inline {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", d.contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, d.filename))
	// Uploaded content is never to be sniffed into something executable.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(d.data); err != nil {
		log.Printf("write response: %v", err)
	}
}
