package handlers

import (
	"context"
	"net/http"
	"os/exec"
	"time"
)

// HealthHandler handles GET /health — returns server liveness status as JSON.
// Used by Docker HEALTHCHECK and load-balancer probes.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pythonOK := exec.CommandContext(ctx, "python3", "--version").Run() == nil

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"python":    pythonOK,
		"version":   "1.0.0",
	})
}
