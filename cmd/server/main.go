// Command server runs the ImageCompressor-DCT HTTP API and serves the web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"imagecompressor-dct/internal/handlers"
)

const (
	// readHeaderTimeout stops clients that open a connection and never
	// finish sending their request headers from holding it forever.
	readHeaderTimeout = 10 * time.Second

	// The body and response limits are generous: an upload can be 10 MB
	// over a slow link, and a benchmark starts Python interpreters.
	readTimeout  = 60 * time.Second
	writeTimeout = 3 * time.Minute
	idleTimeout  = 2 * time.Minute

	// shutdownGrace is how long in-flight requests get to finish on exit.
	shutdownGrace = 15 * time.Second
)

func main() {
	port := flag.Int("port", envInt("SERVER_PORT", 8080), "HTTP port to listen on")
	webDir := flag.String("web", "./web", "directory with the static web UI")
	flag.Parse()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           routes(*webDir),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Serve in the background so this goroutine can wait for a signal.
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on http://localhost%s", server.Addr)
		serveErr <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		// ListenAndServe only returns on failure here, e.g. port in use.
		log.Fatalf("server failed: %v", err)
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
	}

	// Stop accepting connections and let running requests finish.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("shutdown: %v", err)
	}
}

// routes builds the request multiplexer: the API, the health check and, if
// the directory exists, the static web UI at the root.
func routes(webDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", handlers.HealthHandler)

	mux.HandleFunc("/api/upload", handlers.UploadHandler)
	mux.HandleFunc("/api/compress", handlers.CompressHandler)
	mux.HandleFunc("/api/benchmark", handlers.BenchmarkHandler)
	mux.HandleFunc("/api/formats", handlers.FormatsHandler)
	mux.HandleFunc("/api/download/", handlers.DownloadHandler)
	mux.HandleFunc("/api/export", handlers.ExportHandler)

	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(webDir)))
	} else {
		log.Printf("web UI directory %q not found; serving the API only", webDir)
	}
	return mux
}

// envInt reads an integer environment variable, returning fallback if it is
// unset or not a number.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
