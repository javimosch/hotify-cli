package main

// Server-side handlers for app-specific remote operations.
//
// Routes registered in server.go:
//   POST /api/remote/apps/{id}/basic-auth  — manage basic auth credentials
//   POST /api/remote/apps/{id}/setup-traefik  — configure Traefik for a specific app
//   POST /api/remote/apps/{id}/setup-dns     — configure DNS for a specific app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// registerAppRemoteRoutes registers app-specific remote API routes.
// Called from startServer() in server.go.
func registerAppRemoteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/apps/", authMiddleware(handleAppRemoteAPI))
}

// handleAppRemoteAPI handles app-specific remote operations.
// URL pattern: /api/remote/apps/{id}/{action}
// Supported actions: basic-auth, setup-traefik, setup-dns, config-setup, config
func handleAppRemoteAPI(w http.ResponseWriter, r *http.Request) {
	// Extract app ID and action from URL path
	// URL format: /api/remote/apps/{id}/{action}
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 6 {
		http.Error(w, "Invalid URL path", http.StatusBadRequest)
		return
	}
	appID := pathParts[4]
	action := pathParts[5]

	switch action {
	case "basic-auth":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleBasicAuthRemoteAPI(w, r, appID)
	case "setup-traefik":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSetupTraefikRemoteAPI(w, r, appID)
	case "setup-dns":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleSetupDNSRemoteAPI(w, r, appID)
	case "config-setup":
		handleAppConfigRemoteAPI(w, r, appID, "config-setup")
	case "config":
		handleAppConfigRemoteAPI(w, r, appID, "config")
	case "prune":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleRemotePruneAppAPI(w, r, appID)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// handleBasicAuthRemoteAPI handles basic auth operations remotely.
//
// Request body:
//
//	{
//	  "action": "add|remove|list",
//	  "user": "username (for add/remove)",
//	  "password": "plaintext (for add)",
//	  "hash": "pre-hashed entry (for add, skips password)"
//	}
func handleBasicAuthRemoteAPI(w http.ResponseWriter, r *http.Request, appID string) {
	var payload struct {
		Action   string `json:"action"`
		User     string `json:"user"`
		Password string `json:"password"`
		Hash     string `json:"hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if payload.Action == "" {
		http.Error(w, "action is required", http.StatusBadRequest)
		return
	}

	config, err := loadConfig()
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth failed (config load): %v", err),
			Success:   false,
		})
		http.Error(w, "Config load failed", http.StatusInternalServerError)
		return
	}

	app := findApp(config, appID)
	if app == nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth failed (app not found): %s", appID),
			Success:   false,
		})
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	switch payload.Action {
	case "list":
		masked := make([]string, len(app.BasicAuth))
		for i, e := range app.BasicAuth {
			parts := strings.SplitN(e, ":", 2)
			if len(parts) == 2 {
				masked[i] = parts[0] + ":***"
			} else {
				masked[i] = e
			}
		}
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventDeploy,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth list for %s", appID),
			Success:   true,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"app_id":  appID,
			"count":   len(app.BasicAuth),
			"users":   masked,
			"enabled": len(app.BasicAuth) > 0,
		})

	case "add":
		var entry string
		if payload.Hash != "" {
			if !strings.Contains(payload.Hash, ":") {
				auditLogger.LogEvent(AuditEvent{
					EventType: AuditEventAuthFailed,
					TokenName: r.Header.Get("X-API-Key-Name"),
					Details:   "Basic auth add failed: invalid hash format",
					Success:   false,
				})
				http.Error(w, "Hash must be in htpasswd format: user:$apr1$...", http.StatusBadRequest)
				return
			}
			entry = payload.Hash
		} else {
			if payload.User == "" || payload.Password == "" {
				auditLogger.LogEvent(AuditEvent{
					EventType: AuditEventAuthFailed,
					TokenName: r.Header.Get("X-API-Key-Name"),
					Details:   "Basic auth add failed: missing user or password",
					Success:   false,
				})
				http.Error(w, "User and password required (or hash)", http.StatusBadRequest)
				return
			}
			var hashErr error
			entry, hashErr = HtpasswdEntry(payload.User, payload.Password)
			if hashErr != nil {
				auditLogger.LogEvent(AuditEvent{
					EventType: AuditEventAuthFailed,
					TokenName: r.Header.Get("X-API-Key-Name"),
					Details:   fmt.Sprintf("Basic auth add failed: %v", hashErr),
					Success:   false,
				})
				http.Error(w, "Password hashing failed", http.StatusInternalServerError)
				return
			}
		}

		entryUser := strings.SplitN(entry, ":", 2)[0]
		newAuth := make([]string, 0, len(app.BasicAuth)+1)
		replaced := false
		for _, e := range app.BasicAuth {
			if strings.SplitN(e, ":", 2)[0] == entryUser {
				replaced = true
				continue
			}
			newAuth = append(newAuth, e)
		}
		newAuth = append(newAuth, entry)

		for i := range config.Apps {
			if config.Apps[i].ID == appID {
				config.Apps[i].BasicAuth = newAuth
				break
			}
		}
		if err := saveConfig(config); err != nil {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: r.Header.Get("X-API-Key-Name"),
				Details:   fmt.Sprintf("Basic auth add failed (config save): %v", err),
				Success:   false,
			})
			http.Error(w, "Config save failed", http.StatusInternalServerError)
			return
		}

		action := "added"
		if replaced {
			action = "updated"
		}
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventDeploy,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth %s for %s: %s", action, appID, entryUser),
			Success:   true,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"app_id":  appID,
			"user":    entryUser,
			"action":  action,
			"count":   len(newAuth),
		})

	case "remove":
		if payload.User == "" {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: r.Header.Get("X-API-Key-Name"),
				Details:   "Basic auth remove failed: missing user",
				Success:   false,
			})
			http.Error(w, "User required", http.StatusBadRequest)
			return
		}

		newAuth := make([]string, 0, len(app.BasicAuth))
		found := false
		for _, e := range app.BasicAuth {
			if strings.SplitN(e, ":", 2)[0] == payload.User {
				found = true
				continue
			}
			newAuth = append(newAuth, e)
		}
		if !found {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: r.Header.Get("X-API-Key-Name"),
				Details:   fmt.Sprintf("Basic auth remove failed (user not found): %s", payload.User),
				Success:   false,
			})
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		for i := range config.Apps {
			if config.Apps[i].ID == appID {
				config.Apps[i].BasicAuth = newAuth
				break
			}
		}
		if err := saveConfig(config); err != nil {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: r.Header.Get("X-API-Key-Name"),
				Details:   fmt.Sprintf("Basic auth remove failed (config save): %v", err),
				Success:   false,
			})
			http.Error(w, "Config save failed", http.StatusInternalServerError)
			return
		}

		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventDeploy,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth removed for %s: %s", appID, payload.User),
			Success:   true,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":         true,
			"app_id":          appID,
			"user":            payload.User,
			"action":          "removed",
			"remaining_count": len(newAuth),
			"auth_enabled":    len(newAuth) > 0,
		})

	default:
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Basic auth failed (unknown action): %s", payload.Action),
			Success:   false,
		})
		http.Error(w, "Unknown action", http.StatusBadRequest)
	}
}

// handleSetupTraefikRemoteAPI handles per-app Traefik configuration remotely.
//
// Request body:
//
//	{
//	  "challenge_type": "http|dns",
//	  "no_redirect": false
//	}
func handleSetupTraefikRemoteAPI(w http.ResponseWriter, r *http.Request, appID string) {
	var payload struct {
		ChallengeType string `json:"challenge_type"`
		NoRedirect    bool   `json:"no_redirect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var ct TraefikChallengeType
	switch payload.ChallengeType {
	case "dns":
		ct = ChallengeDNS
	case "http", "":
		ct = ChallengeHTTP
	default:
		http.Error(w, "Invalid challenge_type: must be 'http' or 'dns'", http.StatusBadRequest)
		return
	}

	var err error
	if payload.NoRedirect {
		// Use explicit redirect control when flag is provided
		err = setupTraefikForAppWithChallengeAndRedirect(appID, ct, false, false)
	} else {
		// Use smart redirect handling by default
		err = setupTraefikForAppWithChallenge(appID, ct, false)
	}
	
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Setup Traefik failed for %s: %v", appID, err),
			Success:   false,
		})
		http.Error(w, "Traefik setup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Traefik configured for app: %s (challenge: %s)", appID, ct),
		Success:   true,
	})

	// Include proxy fields in the response so callers can confirm routing.
	var backendURL, pathPrefix string
	if cfg, err := loadConfig(); err == nil {
		if app := findApp(cfg, appID); app != nil {
			backendURL = app.BackendURL
			pathPrefix = app.PathPrefix
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"app_id":           appID,
		"challenge_type":   string(ct),
		"redirect_enabled": !payload.NoRedirect,
		"backend_url":      backendURL,
		"path_prefix":      pathPrefix,
		"action":           "traefik_configured",
	})
}

// handleSetupDNSRemoteAPI handles per-app DNS configuration remotely.
//
// Request body:
//
//	{
//	  "ip": "1.2.3.4" (optional, auto-detected if omitted)
//	}
func handleSetupDNSRemoteAPI(w http.ResponseWriter, r *http.Request, appID string) {
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resolvedIP, warn, err := resolveServerIP(payload.IP)
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Setup DNS failed for %s (IP resolution): %v", appID, err),
			Success:   false,
		})
		http.Error(w, "IP resolution failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := setupDNSForApp(appID, resolvedIP); err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Setup DNS failed for %s: %v", appID, err),
			Success:   false,
		})
		http.Error(w, "DNS setup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("DNS configured for app: %s → %s", appID, resolvedIP),
		Success:   true,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"app_id":  appID,
		"ip":      resolvedIP,
		"action":  "dns_configured",
		"warning": warn,
	})
}