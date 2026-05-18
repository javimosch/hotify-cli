package main

// Server-side handlers for Docker Compose deployment API endpoints.
//
// Routes registered in server.go:
//   POST /api/compose/volume-init  — populate a Docker named volume with uploaded files

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// registerComposeRoutes registers compose-specific API routes onto the given ServeMux.
// Called from startServer() in server.go.
func registerComposeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/compose/volume-init", authMiddleware(handleComposeVolumeInitAPI))
}

// handleComposeVolumeInitAPI populates a Docker named volume with the uploaded directory.
//
// Request body:
//
//	{
//	  "app_id":      "cir-doc-gen",
//	  "volume_name": "cir-webui",           // suffix: full volume = <app_id>_<volume_name>
//	  "data":        "<base64-encoded tar.gz>"
//	}
//
// The handler writes the tar.gz contents into
// /var/lib/docker/volumes/<app_id>_<volume_name>/_data/.
//
// NOTE: This requires the hotify daemon process to have write access to
// /var/lib/docker/volumes/ (root or Docker group membership on the remote host).
func handleComposeVolumeInitAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		AppID      string `json:"app_id"`
		VolumeName string `json:"volume_name"`
		Data       string `json:"data"` // base64-encoded tar.gz
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if payload.AppID == "" || payload.VolumeName == "" || payload.Data == "" {
		http.Error(w, "app_id, volume_name, and data are required", http.StatusBadRequest)
		return
	}

	// Decode payload
	tarData, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		http.Error(w, "Error decoding data: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build Docker volume _data path
	fullVolume := fmt.Sprintf("%s_%s", payload.AppID, payload.VolumeName)
	volumeDataPath := filepath.Join("/var/lib/docker/volumes", fullVolume, "_data")

	if err := populateVolumeData(tarData, volumeDataPath); err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Volume init failed for %s: %v", fullVolume, err),
			Success:   false,
		})
		http.Error(w, "Volume init failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Volume init successful: %s → %s", fullVolume, volumeDataPath),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"message":     "Volume initialized",
		"volume":      fullVolume,
		"volume_path": volumeDataPath,
	})
}

// populateVolumeData extracts a tar.gz archive into destPath.
// It ensures the destination directory exists first.
func populateVolumeData(tarData []byte, destPath string) error {
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("error creating volume data path: %v", err)
	}

	tmpFile := fmt.Sprintf("/tmp/vol_init_%d.tar.gz", time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, tarData, 0644); err != nil {
		return fmt.Errorf("error writing temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	out, err := exec.Command("tar", "-xzf", tmpFile, "-C", destPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("extraction failed: %v — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
