// Package main is the entry point for the ImageCompressor-DCT HTTP server.
// It registers all API routes and serves the static web UI.
package main

import (
	"flag"
	"fmt"
	"imagecompressor-dct/internal/handlers"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	port := flag.Int("port", envInt("SERVER_PORT", 8080), "HTTP server port")
	flag.Parse()

	mux := http.NewServeMux()

	// Infra routes.
	mux.HandleFunc("/health", handlers.HealthHandler)

	// API routes.
	mux.HandleFunc("/api/upload", handlers.UploadHandler)
	mux.HandleFunc("/api/compress", handlers.CompressHandler)
	mux.HandleFunc("/api/benchmark", handlers.BenchmarkHandler)
	mux.HandleFunc("/api/formats", handlers.FormatsHandler)
	mux.HandleFunc("/api/download/", handlers.DownloadHandler)
	mux.HandleFunc("/api/export", handlers.ExportHandler)

	// Serve static web UI from the web/ directory.
	webDir := "./web"
	if _, err := os.Stat(webDir); err == nil {
		mux.Handle("/", http.FileServer(http.Dir(webDir)))
	}

	address := fmt.Sprintf(":%d", *port)
	log.Printf("ImageCompressor-DCT server starting on http://localhost%s", address)

	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// envInt reads an integer from an environment variable, falling back to def.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
