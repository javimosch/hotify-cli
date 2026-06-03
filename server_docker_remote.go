package main

// Remote API handlers for Docker management.
//
// Routes registered via registerDockerRemoteRoutes (called from startServer):
//   GET  /api/remote/docker/containers         — list containers
//   POST /api/remote/docker/containers/{id}/start
//   POST /api/remote/docker/containers/{id}/stop
//   POST /api/remote/docker/containers/{id}/restart
//   GET  /api/remote/docker/containers/{id}/status
//   GET  /api/remote/docker/containers/{id}/logs
//   POST /api/remote/docker/enable-traefik
//   POST /api/remote/docker/disable-traefik

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func registerDockerRemoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/docker/", authMiddleware(handleDockerRemoteAPI))
}

func handleDockerRemoteAPI(w http.ResponseWriter, r *http.Request) {
	// /api/remote/docker/<sub>
	path := strings.TrimPrefix(r.URL.Path, "/api/remote/docker/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	switch parts[0] {
	case "containers":
		handleDockerContainersAPI(w, r, parts[1:])
	case "enable-traefik":
		handleDockerEnableTraefikRemoteAPI(w, r)
	case "disable-traefik":
		handleDockerDisableTraefikRemoteAPI(w, r)
	default:
		http.Error(w, "Unknown docker remote action", http.StatusNotFound)
	}
}

func handleDockerContainersAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	// GET /containers → list
	if len(parts) == 0 {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		containers, err := dockerList()
		if err != nil {
			http.Error(w, "Docker list failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"containers": containers,
			"count":      len(containers),
		})
		return
	}

	// /containers/{id}/{action}
	containerID := parts[0]
	if len(parts) < 2 {
		// GET /containers/{id} → status
		container, err := dockerStatus(containerID)
		if err != nil {
			http.Error(w, "Container not found: "+err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"id":        container.ID,
			"name":      container.Name,
			"image":     container.Image,
			"status":    container.Status,
			"ports":     container.Ports,
			"labels":    container.Labels,
		})
		return
	}

	action := parts[1]
	switch action {
	case "start":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := dockerStart(containerID); err != nil {
			http.Error(w, "Start failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "container_id": containerID, "action": "started",
		})

	case "stop":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := dockerStop(containerID); err != nil {
			http.Error(w, "Stop failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "container_id": containerID, "action": "stopped",
		})

	case "restart":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := dockerRestart(containerID); err != nil {
			http.Error(w, "Restart failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "container_id": containerID, "action": "restarted",
		})

	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		container, err := dockerStatus(containerID)
		if err != nil {
			http.Error(w, "Status failed: "+err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "id": container.ID, "name": container.Name,
			"image": container.Image, "status": container.Status,
			"ports": container.Ports, "labels": container.Labels,
		})

	case "logs":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tail := 50
		if t := r.URL.Query().Get("tail"); t != "" {
			if n, err := strconv.Atoi(t); err == nil && n > 0 {
				tail = n
			}
		}
		logs, err := dockerLogs(containerID, tail)
		if err != nil {
			http.Error(w, "Logs failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true, "container_id": containerID, "logs": logs,
		})

	default:
		http.Error(w, fmt.Sprintf("Unknown container action: %s", action), http.StatusNotFound)
	}
}

func handleDockerEnableTraefikRemoteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := enableDockerProvider(); err != nil {
		http.Error(w, "Enable traefik failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "action": "docker_provider_enabled",
	})
}

func handleDockerDisableTraefikRemoteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := disableDockerProvider(); err != nil {
		http.Error(w, "Disable traefik failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true, "action": "docker_provider_disabled",
	})
}
