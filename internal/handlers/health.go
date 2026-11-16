package handlers

import (
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Version is reported by the health endpoint. It can be set at build time
// with -ldflags "-X imagecompressor-dct/internal/handlers.Version=...".
var Version = "dev"

// pythonAvailable looks for the interpreter once. Health checks arrive every
// few seconds, and the answer does not change while the process runs.
var pythonAvailable = sync.OnceValue(func() bool {
	_, err := exec.LookPath("python3")
	return err == nil
})

// HealthHandler handles GET /health for container and load balancer probes.
// "python" tells whether the optional Python benchmarks can run.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"python":    pythonAvailable(),
		"version":   Version,
	})
}
