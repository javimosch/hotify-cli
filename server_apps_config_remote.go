package main

// Remote API handlers for app config management (setup/add/edit/remove/list).
//
// Routes (registered in registerAppRemoteRoutes):
//   POST /api/remote/apps/{id}/config-setup   — upsert app
//   DELETE /api/remote/apps/{id}/config       — remove app
//   GET  /api/remote/apps/{id}/config         — get app details
//
// List is already served by GET /api/apps (existing endpoint).

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleAppConfigRemoteAPI handles app config operations: setup, remove, get.
// URL: /api/remote/apps/{id}/config[/setup]  or /api/remote/apps/{id}/config
func handleAppConfigRemoteAPI(w http.ResponseWriter, r *http.Request, appID string, action string) {
	switch action {
	case "config-setup":
		handleRemoteAppSetupAPI(w, r, appID)
	case "config":
		switch r.Method {
		case http.MethodGet:
			handleRemoteAppGetAPI(w, r, appID)
		case http.MethodDelete:
			handleRemoteAppRemoveAPI(w, r, appID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Unknown config action", http.StatusNotFound)
	}
}

// handleRemoteAppSetupAPI creates or updates an app config.
//
// POST /api/remote/apps/{id}/config-setup
// Body: { name, domain, port, cmd, source, compose_file, compose_path, backend_url }
func handleRemoteAppSetupAPI(w http.ResponseWriter, r *http.Request, appID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Name        string `json:"name"`
		Domain      string `json:"domain"`
		Port        int    `json:"port"`
		Command     string `json:"cmd"`
		Source      string `json:"source"`
		ComposeFile string `json:"compose_file"`
		ComposePath string `json:"compose_path"`
		BackendURL  string `json:"backend_url"`
		PathPrefix  string `json:"path_prefix"`
		SetupDNS    bool   `json:"setup_dns"`
		IP          string `json:"ip"`
		FullDomain  bool   `json:"full_domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		http.Error(w, "Config load failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	warnings := []string{}
	existingIdx := -1
	for i, app := range config.Apps {
		if app.ID == appID {
			existingIdx = i
			break
		}
	}

	if existingIdx >= 0 {
		// Update existing
		app := config.Apps[existingIdx]
		if payload.Name != "" {
			app.Name = payload.Name
		}
		if payload.Domain != "" {
			if payload.FullDomain {
				app.Domain = payload.Domain
			} else {
				app.Domain = fmt.Sprintf("%s.%s", payload.Domain, config.Domain)
			}
		}
		if payload.Port != 0 {
			app.Port = payload.Port
		}
		if payload.Command != "" {
			app.Command = payload.Command
		}
		if payload.Source != "" {
			app.Source = payload.Source
		}
		if payload.ComposeFile != "" {
			app.ComposeFile = payload.ComposeFile
		}
		if payload.ComposePath != "" {
			app.ComposePath = payload.ComposePath
		}
		if payload.BackendURL != "" {
			app.BackendURL = payload.BackendURL
		}
		if payload.PathPrefix != "" {
			app.PathPrefix = payload.PathPrefix
		}
		config.Apps[existingIdx] = app
	} else {
		// New app requires name, domain, port; command can be omitted when
		// backend_url is provided for externally managed proxy services.
		if payload.Name == "" || payload.Domain == "" || payload.Port == 0 || (payload.Command == "" && payload.BackendURL == "") {
			http.Error(w, "New app requires name, domain, port, and cmd (or backend_url for proxy apps)", http.StatusBadRequest)
			return
		}
		var fullDomain string
		if payload.FullDomain {
			fullDomain = payload.Domain
		} else {
			fullDomain = fmt.Sprintf("%s.%s", payload.Domain, config.Domain)
		}
		config.Apps = append(config.Apps, App{
			ID:          appID,
			Name:        payload.Name,
			Domain:      fullDomain,
			Port:        payload.Port,
			Command:     payload.Command,
			Source:      payload.Source,
			Status:      "stopped",
			ComposeFile: payload.ComposeFile,
			ComposePath: payload.ComposePath,
			BackendURL:  payload.BackendURL,
			PathPrefix:  payload.PathPrefix,
		})
		existingIdx = len(config.Apps) - 1
	}

	if err := saveConfig(config); err != nil {
		http.Error(w, "Config save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Optional DNS setup
	if payload.SetupDNS {
		app := config.Apps[existingIdx]
		resolvedIP, warn, resolveErr := resolveServerIP(payload.IP)
		if resolveErr != nil {
			warnings = append(warnings, "DNS setup skipped: "+resolveErr.Error())
		} else {
			if warn != "" {
				warnings = append(warnings, warn)
			}
			zoneID, zErr := getZoneID(app.Domain, config.CloudflareToken, config.AdminEmail)
			if zErr != nil {
				warnings = append(warnings, "DNS zone lookup failed: "+zErr.Error())
			} else if dnsErr := setupDNSRecord(app.Domain, resolvedIP, zoneID, config.CloudflareToken, config.AdminEmail); dnsErr != nil {
				warnings = append(warnings, "DNS record error: "+dnsErr.Error())
			}
		}
	}

	app := config.Apps[existingIdx]
	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("App config upserted: %s", appID),
		Success:   true,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"app_id":       appID,
		"name":         app.Name,
		"domain":       app.Domain,
		"port":         app.Port,
		"compose_file": app.ComposeFile,
		"compose_path": app.ComposePath,
		"backend_url":  app.BackendURL,
		"path_prefix":  app.PathPrefix,
		"warnings":     warnings,
	})
}

// handleRemoteAppRemoveAPI removes an app from config.
//
// DELETE /api/remote/apps/{id}/config
func handleRemoteAppRemoveAPI(w http.ResponseWriter, r *http.Request, appID string) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, "Config load failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	updated := make([]App, 0, len(config.Apps))
	for _, app := range config.Apps {
		if app.ID == appID {
			found = true
		} else {
			updated = append(updated, app)
		}
	}

	if !found {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	config.Apps = updated
	if err := saveConfig(config); err != nil {
		http.Error(w, "Config save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("App config removed: %s", appID),
		Success:   true,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"app_id":  appID,
		"action":  "removed",
		"warnings": []string{
			"DNS record was NOT removed from Cloudflare",
			"Traefik routing config was NOT cleaned up",
		},
	})
}

// handleRemoteAppGetAPI returns a single app's config.
//
// GET /api/remote/apps/{id}/config
func handleRemoteAppGetAPI(w http.ResponseWriter, r *http.Request, appID string) {
	config, err := loadConfig()
	if err != nil {
		http.Error(w, "Config load failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	app := findApp(config, appID)
	if app == nil {
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"id":           app.ID,
		"name":         app.Name,
		"domain":       app.Domain,
		"port":         app.Port,
		"cmd":          app.Command,
		"source":       app.Source,
		"status":       app.Status,
		"compose_file": app.ComposeFile,
		"compose_path": app.ComposePath,
		"backend_url":  app.BackendURL,
		"path_prefix":  app.PathPrefix,
	})
}


