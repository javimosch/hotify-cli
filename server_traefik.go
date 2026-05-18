package main

// Server-side handlers for Traefik installation and management.
//
// Routes registered in server.go:
//   POST /api/traefik-system/status  — check Traefik installation status
//   POST /api/traefik-system/install  — install Traefik (with --force flag)
//   POST /api/traefik-system/remove   — remove Traefik installation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
)

// registerTraefikSystemRoutes registers Traefik system management API routes.
// Called from startServer() in server.go.
func registerTraefikSystemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/traefik-system/status", authMiddleware(handleTraefikSystemStatusAPI))
	mux.HandleFunc("/api/traefik-system/install", authMiddleware(handleTraefikSystemInstallAPI))
	mux.HandleFunc("/api/traefik-system/remove", authMiddleware(handleTraefikSystemRemoveAPI))
}

// handleTraefikSystemStatusAPI checks the current Traefik installation status.
//
// Request body: empty
// Response: TraefikStatus JSON
func handleTraefikSystemStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := checkTraefikStatusLocal()
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik status check failed: %v", err),
			Success:   false,
		})
		http.Error(w, "Status check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   "Traefik status check successful",
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  status,
	})
}

// handleTraefikSystemInstallAPI performs idempotent Traefik installation.
//
// Request body:
//
//	{
//	  "force": false  // force reinstall if already installed
//	}
//
// Response: installation result with actions taken
func handleTraefikSystemInstallAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Force bool `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Check current status
	status, err := checkTraefikStatusLocal()
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik install failed (status check): %v", err),
			Success:   false,
		})
		http.Error(w, "Status check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if status.Installed && !payload.Force {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   "Traefik already installed, use --force to reinstall",
			Success:   false,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Traefik is already installed",
			"code":    ExitTraefikAlreadyInstalled,
			"status":  status,
		})
		return
	}

	actions := []string{}

	// Install binary
	if !status.Installed || payload.Force {
		if err := installTraefikBinaryLocal(); err != nil {
			auditLogger.LogEvent(AuditEvent{
				EventType: AuditEventAuthFailed,
				TokenName: r.Header.Get("X-API-Key-Name"),
				Details:   fmt.Sprintf("Traefik install failed (binary): %v", err),
				Success:   false,
			})
			http.Error(w, "Binary installation failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		actions = append(actions, "installed_binary")
	}

	// Create config dir
	if err := createTraefikConfigDirLocal(); err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik install failed (config dir): %v", err),
			Success:   false,
		})
		http.Error(w, "Config directory creation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	actions = append(actions, "created_config_dir")

	// Setup systemd service
	if err := setupTraefikSystemdServiceLocal(); err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik install failed (systemd): %v", err),
			Success:   false,
		})
		http.Error(w, "Systemd service setup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	actions = append(actions, "setup_systemd_service")

	// Start service
	if err := startTraefikServiceLocal(); err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik install failed (start): %v", err),
			Success:   false,
		})
		http.Error(w, "Service start failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	actions = append(actions, "started_service")

	// Get final status
	finalStatus, _ := checkTraefikStatusLocal()

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Traefik install successful: %s", strings.Join(actions, ", ")),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"status":        finalStatus,
		"actions_taken":  actions,
		"traefik_version": finalStatus.Version,
	})
}

// handleTraefikSystemRemoveAPI removes Traefik installation.
//
// Request body: empty
// Response: removal result with actions taken
func handleTraefikSystemRemoveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := checkTraefikStatusLocal()
	if err != nil {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   fmt.Sprintf("Traefik remove failed (status check): %v", err),
			Success:   false,
		})
		http.Error(w, "Status check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !status.Installed {
		auditLogger.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: r.Header.Get("X-API-Key-Name"),
			Details:   "Traefik not installed, cannot remove",
			Success:   false,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Traefik is not installed",
			"code":    ExitTraefikNotInstalled,
		})
		return
	}

	actions := []string{}

	// Stop and disable service
	if status.SystemdEnabled {
		runSudoCommand("systemctl stop traefik")
		runSudoCommand("systemctl disable traefik")
		actions = append(actions, "stopped_service")
	}

	// Remove systemd service file
	runSudoCommand("rm -f /etc/systemd/system/traefik.service")
	runSudoCommand("systemctl daemon-reload")
	actions = append(actions, "removed_systemd_service")

	// Remove binary
	runSudoCommand("rm -f /usr/local/bin/traefik")
	actions = append(actions, "removed_binary")

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Traefik remove successful: %s", strings.Join(actions, ", ")),
		Success:   true,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"actions_taken":  actions,
		"note":          "Configuration directory preserved at /etc/traefik",
	})
}

// ─── Local helpers (run commands on the server) ───────────────────────────────

// TraefikStatus represents the current Traefik installation status
type TraefikStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	Status         string `json:"status,omitempty"`
	BinaryPath     string `json:"binary_path,omitempty"`
	ConfigDir      string `json:"config_dir,omitempty"`
	ServiceName    string `json:"service_name,omitempty"`
	SystemdEnabled bool   `json:"systemd_enabled,omitempty"`
}

// checkTraefikStatusLocal checks Traefik installation status on the local server.
func checkTraefikStatusLocal() (TraefikStatus, error) {
	status := TraefikStatus{
		ConfigDir:   "/etc/traefik",
		ServiceName: "traefik.service",
	}

	// Check if Traefik binary exists
	_, err := exec.Command("which", "traefik").CombinedOutput()
	if err == nil {
		status.Installed = true
		status.BinaryPath = "/usr/local/bin/traefik"

		// Get version
		output, _ := exec.Command("traefik", "version").CombinedOutput()
		if output != nil {
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				status.Version = strings.TrimSpace(lines[0])
			}
		}
	}

	// Check systemd service status
	output, _ := exec.Command("systemctl", "is-active", "traefik").CombinedOutput()
	if output != nil {
		status.Status = strings.TrimSpace(string(output))
	}

	// Check if systemd is enabled
	output, _ = exec.Command("systemctl", "is-enabled", "traefik").CombinedOutput()
	if output != nil {
		status.SystemdEnabled = strings.TrimSpace(string(output)) == "enabled"
	}

	return status, nil
}

// installTraefikBinaryLocal downloads and installs Traefik binary.
func installTraefikBinaryLocal() error {
	installCmd := exec.Command("bash", "-c", "curl -sSL https://raw.githubusercontent.com/traefik/traefik/v2.9/scripts/install-on-linux.sh | sudo bash -s v2.9.0")
	output, err := installCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install Traefik: %v\nOutput: %s", err, string(output))
	}

	// Verify installation
	_, err = exec.Command("which", "traefik").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Traefik binary not found after installation")
	}

	return nil
}

// createTraefikConfigDirLocal creates Traefik configuration directory and basic config.
func createTraefikConfigDirLocal() error {
	if err := runSudoCommand("mkdir -p /etc/traefik"); err != nil {
		return err
	}

	// Create basic traefik.yml if it doesn't exist
	output, err := exec.Command("bash", "-c", "test -f /etc/traefik/traefik.yml").CombinedOutput()
	if err != nil {
		// Create basic configuration
		basicConfig := `api:
  dashboard: true
  insecure: true

entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

certificatesResolvers:
  cloudflare:
    acme:
      email: your-email@example.com
      storage: /etc/traefik/acme.json
      dnsChallenge:
        provider: cloudflare
        resolvers:
          - "1.1.1.1:53"
          - "1.0.0.1:53"
`
		createCmd := fmt.Sprintf("sudo tee /etc/traefik/traefik.yml > /dev/null << 'EOF'\n%s\nEOF", basicConfig)
		output, err = exec.Command("bash", "-c", createCmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create traefik.yml: %v\nOutput: %s", err, string(output))
		}
	}

	return nil
}

// setupTraefikSystemdServiceLocal creates and enables systemd service.
func setupTraefikSystemdServiceLocal() error {
	serviceContent := `[Unit]
Description=Traefik
Documentation=https://docs.traefik.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/traefik --configFile=/etc/traefik/traefik.yml
Restart=always
WatchdogSec=1s
TimeoutStopSec=5s

[Install]
WantedBy=multi-user.target
`

	createCmd := fmt.Sprintf("sudo tee /etc/systemd/system/traefik.service > /dev/null << 'EOF'\n%s\nEOF", serviceContent)
	output, err := exec.Command("bash", "-c", createCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create service file: %v\nOutput: %s", err, string(output))
	}

	// Reload systemd
	if err := runSudoCommand("systemctl daemon-reload"); err != nil {
		return err
	}

	// Enable service
	if err := runSudoCommand("systemctl enable traefik"); err != nil {
		return err
	}

	return nil
}

// startTraefikServiceLocal starts the Traefik systemd service.
func startTraefikServiceLocal() error {
	if err := runSudoCommand("systemctl restart traefik"); err != nil {
		return err
	}

	// Wait for service to be active
	for i := 0; i < 10; i++ {
		output, _ := exec.Command("systemctl", "is-active", "traefik").CombinedOutput()
		if strings.TrimSpace(string(output)) == "active" {
			return nil
		}
	}

	return fmt.Errorf("Traefik service did not become active")
}

// runSudoCommand runs a command with sudo.
func runSudoCommand(cmd string) error {
	fullCmd := fmt.Sprintf("sudo %s", cmd)
	output, err := exec.Command("bash", "-c", fullCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %s\nOutput: %s", cmd, string(output))
	}
	return nil
}
