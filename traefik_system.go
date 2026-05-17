package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TraefikSystemManager handles Traefik installation and management
type TraefikSystemManager struct {
	Target *Remote
}

// TraefikStatus represents the current Traefik installation status
type TraefikStatus struct {
	Installed    bool   `json:"installed"`
	Version      string `json:"version,omitempty"`
	Status       string `json:"status,omitempty"`
	BinaryPath   string `json:"binary_path,omitempty"`
	ConfigDir    string `json:"config_dir,omitempty"`
	ServiceName  string `json:"service_name,omitempty"`
	SystemdEnabled bool `json:"systemd_enabled,omitempty"`
}

// NewTraefikSystemManager creates a new Traefik system manager
func NewTraefikSystemManager(target *Remote) *TraefikSystemManager {
	return &TraefikSystemManager{
		Target: target,
	}
}

// CheckStatus checks the current Traefik installation status
func (t *TraefikSystemManager) CheckStatus() (TraefikStatus, error) {
	status := TraefikStatus{
		ConfigDir:   "/etc/traefik",
		ServiceName: "traefik.service",
	}

	// Check if Traefik binary exists
	_, err := t.runCommand("which traefik")
	if err == nil {
		status.Installed = true
		status.BinaryPath = "/usr/local/bin/traefik"

		// Get version
		versionOutput, _ := t.runCommand("traefik version")
		if versionOutput != "" {
			lines := strings.Split(versionOutput, "\n")
			if len(lines) > 0 {
				status.Version = strings.TrimSpace(lines[0])
			}
		}
	}

	// Check systemd service status
	serviceOutput, _ := t.runCommand("systemctl is-active traefik")
	if serviceOutput != "" {
		status.Status = strings.TrimSpace(serviceOutput)
	}

	// Check if systemd is enabled
	enabledOutput, _ := t.runCommand("systemctl is-enabled traefik")
	status.SystemdEnabled = strings.TrimSpace(enabledOutput) == "enabled"

	return status, nil
}

// Install performs idempotent Traefik installation
func (t *TraefikSystemManager) Install(force bool) (CommandResult, error) {
	result := CommandResult{
		Version: "1.0",
		Success: false,
		Data:    make(map[string]interface{}),
	}

	// Check current status
	status, err := t.CheckStatus()
	if err != nil {
		return result, fmt.Errorf("failed to check Traefik status: %v", err)
	}

	if status.Installed && !force {
		result.Error = &CommandError{
			Code:        ExitTraefikAlreadyInstalled,
			Type:        "traefik_already_installed",
			Message:     "Traefik is already installed",
			Recoverable: true,
			Suggestions: []string{
				"Use --force to reinstall",
				"Use --status to check current installation",
			},
		}
		result.Data["status"] = status
		return result, nil
	}

	actions := []string{}

	// Download and install Traefik binary
	if !status.Installed || force {
		fmt.Fprintf(os.Stderr, "Installing Traefik binary...\n")
		err := t.installTraefikBinary()
		if err != nil {
			result.Error = &CommandError{
				Code:        ExitTraefikInstallFailed,
				Type:        "traefik_install_failed",
				Message:     fmt.Sprintf("Failed to install Traefik binary: %v", err),
				Recoverable: false,
				Suggestions: []string{
					"Check internet connection",
					"Verify download URL is accessible",
					"Check file permissions",
				},
			}
			return result, nil
		}
		actions = append(actions, "installed_binary")
	}

	// Create configuration directory
	fmt.Fprintf(os.Stderr, "Creating configuration directory...\n")
	err = t.createConfigDir()
	if err != nil {
		result.Error = &CommandError{
			Code:        ExitPermissionsError,
			Type:        "permissions_error",
			Message:     fmt.Sprintf("Failed to create config directory: %v", err),
			Recoverable: false,
			Suggestions: []string{
				"Run with sudo privileges",
				"Check disk space",
				"Verify parent directory permissions",
			},
		}
		return result, nil
	}
	actions = append(actions, "created_config_dir")

	// Setup systemd service
	fmt.Fprintf(os.Stderr, "Setting up systemd service...\n")
	err = t.setupSystemdService()
	if err != nil {
		result.Error = &CommandError{
			Code:        ExitTraefikServiceFailed,
			Type:        "traefik_service_failed",
			Message:     fmt.Sprintf("Failed to setup systemd service: %v", err),
			Recoverable: false,
			Suggestions: []string{
				"Check systemd is available",
				"Verify systemd configuration",
				"Check service file permissions",
			},
		}
		return result, nil
	}
	actions = append(actions, "setup_systemd_service")

	// Start Traefik service
	fmt.Fprintf(os.Stderr, "Starting Traefik service...\n")
	err = t.startTraefikService()
	if err != nil {
		result.Error = &CommandError{
			Code:        ExitTraefikServiceFailed,
			Type:        "traefik_service_failed",
			Message:     fmt.Sprintf("Failed to start Traefik service: %v", err),
			Recoverable: true,
			Suggestions: []string{
				"Check Traefik configuration",
				"View logs: journalctl -u traefik -n 50",
				"Validate config: traefik validate /etc/traefik/traefik.yml",
			},
		}
		return result, nil
	}
	actions = append(actions, "started_service")

	// Get final status
	finalStatus, _ := t.CheckStatus()

	result.Success = true
	result.Data["status"] = finalStatus
	result.Data["actions_taken"] = actions
	result.Metadata = map[string]interface{}{
		"traefik_version": finalStatus.Version,
		"install_method":  "binary",
	}

	return result, nil
}

// Remove removes Traefik installation
func (t *TraefikSystemManager) Remove() (CommandResult, error) {
	result := CommandResult{
		Version: "1.0",
		Success: false,
		Data:    make(map[string]interface{}),
	}

	// Check current status
	status, err := t.CheckStatus()
	if err != nil {
		return result, fmt.Errorf("failed to check Traefik status: %v", err)
	}

	if !status.Installed {
		result.Error = &CommandError{
			Code:        ExitTraefikNotInstalled,
			Type:        "traefik_not_installed",
			Message:     "Traefik is not installed",
			Recoverable: false,
			Suggestions: []string{
				"Use --status to verify installation state",
			},
		}
		return result, nil
	}

	actions := []string{}

	// Stop and disable service
	if status.SystemdEnabled {
		fmt.Fprintf(os.Stderr, "Stopping Traefik service...\n")
		_, _ = t.runCommand("sudo systemctl stop traefik")
		_, _ = t.runCommand("sudo systemctl disable traefik")
		actions = append(actions, "stopped_service")
	}

	// Remove systemd service file
	fmt.Fprintf(os.Stderr, "Removing systemd service...\n")
	_, _ = t.runCommand("sudo rm -f /etc/systemd/system/traefik.service")
	_, _ = t.runCommand("sudo systemctl daemon-reload")
	actions = append(actions, "removed_systemd_service")

	// Remove binary
	fmt.Fprintf(os.Stderr, "Removing Traefik binary...\n")
	_, _ = t.runCommand("sudo rm -f /usr/local/bin/traefik")
	actions = append(actions, "removed_binary")

	// Remove configuration directory (optional - keep for safety)
	fmt.Fprintf(os.Stderr, "Configuration directory preserved at /etc/traefik\n")

	result.Success = true
	result.Data["actions_taken"] = actions
	result.Data["note"] = "Configuration directory preserved for safety"

	return result, nil
}

// installTraefikBinary downloads and installs Traefik binary
func (t *TraefikSystemManager) installTraefikBinary() error {
	// Use official Traefik installation script
	installCmd := `curl -sSL https://raw.githubusercontent.com/traefik/traefik/v2.9/scripts/install-on-linux.sh | sudo bash -s v2.9.0`
	_, err := t.runCommand(installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Traefik: %v", err)
	}

	// Verify installation
	_, err = t.runCommand("which traefik")
	if err != nil {
		return fmt.Errorf("Traefik binary not found after installation")
	}

	return nil
}

// createConfigDir creates Traefik configuration directory
func (t *TraefikSystemManager) createConfigDir() error {
	_, err := t.runCommand("sudo mkdir -p /etc/traefik")
	if err != nil {
		return err
	}

	// Create basic traefik.yml if it doesn't exist
	checkCmd := "test -f /etc/traefik/traefik.yml"
	_, err = t.runCommand(checkCmd)
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
		_, err = t.runCommand(createCmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// setupSystemdService creates and enables systemd service
func (t *TraefikSystemManager) setupSystemdService() error {
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
	_, err := t.runCommand(createCmd)
	if err != nil {
		return err
	}

	// Reload systemd
	_, err = t.runCommand("sudo systemctl daemon-reload")
	if err != nil {
		return err
	}

	// Enable service
	_, err = t.runCommand("sudo systemctl enable traefik")
	if err != nil {
		return err
	}

	return nil
}

// startTraefikService starts the Traefik systemd service
func (t *TraefikSystemManager) startTraefikService() error {
	_, err := t.runCommand("sudo systemctl restart traefik")
	if err != nil {
		return err
	}

	// Wait for service to be active
	for i := 0; i < 10; i++ {
		output, _ := t.runCommand("systemctl is-active traefik")
		if strings.TrimSpace(output) == "active" {
			return nil
		}
	}

	return fmt.Errorf("Traefik service did not become active")
}

// runCommand executes a command on the remote target
func (t *TraefikSystemManager) runCommand(cmd string) (string, error) {
	if t.Target == nil {
		// Local execution
		output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
		return string(output), err
	}

	// Determine SSH host
	host := t.Target.SSHHost
	if host == "" {
		// Fallback: infer from HTTP URL
		host = strings.TrimPrefix(t.Target.URL, "http://")
		host = strings.TrimPrefix(host, "https://")
		host = strings.Split(host, ":")[0]
	}

	// Remote execution via SSH
	sshCmd := fmt.Sprintf("ssh %s '%s'", host, cmd)
	output, err := exec.Command("bash", "-c", sshCmd).CombinedOutput()
	return string(output), err
}

// handleTraefikSystem handles the Traefik system management CLI command
func handleTraefikSystem() {
	traefikCmd := flag.NewFlagSet("traefik-system", flag.ExitOnError)
	targetName := traefikCmd.String("target", "", "Target name (uses default if not specified)")
	removeFlag := traefikCmd.Bool("remove", false, "Remove Traefik installation")
	statusFlag := traefikCmd.Bool("status", false, "Check Traefik installation status")
	forceFlag := traefikCmd.Bool("force", false, "Force reinstallation")
	
	// Filter out --human flag before parsing
	filteredArgs := filterHumanFlag(os.Args[2:])
	traefikCmd.Parse(filteredArgs)

	// Get target
	target, err := getActiveTarget(*targetName)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_not_found",
				Message:     fmt.Sprintf("Target not found: %v", err),
				Recoverable: false,
				Suggestions: []string{
					"List available targets: hotify-cli targets --action list",
					"Set default target: hotify-cli targets --action use --name <name>",
				},
			},
		}
		printOutput(result, OutputFormatJSON)
		os.Exit(ExitTargetNotFound)
	}

	// Determine output format (JSON by default, --human for text)
	format := getOutputFormat()

	manager := NewTraefikSystemManager(target)

	// Handle status check
	if *statusFlag {
		status, err := manager.CheckStatus()
		if err != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitGenericFailure,
					Type:        "status_check_failed",
					Message:     fmt.Sprintf("Failed to check status: %v", err),
					Recoverable: false,
				},
			}
			printOutput(result, format)
			os.Exit(ExitGenericFailure)
		}

		result := CommandResult{
			Version: "1.0",
			Success: true,
			Data: map[string]interface{}{
				"status": status,
			},
		}
		printOutput(result, format)
		return
	}

	// Handle removal
	if *removeFlag {
		result, err := manager.Remove()
		if err != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitGenericFailure,
					Type:        "removal_failed",
					Message:     fmt.Sprintf("Removal failed: %v", err),
					Recoverable: false,
				},
			}
			printOutput(result, format)
			os.Exit(ExitGenericFailure)
		}

		printOutput(result, format)
		if result.Success {
			os.Exit(ExitSuccess)
		} else {
			os.Exit(result.Error.Code)
		}
	}

	// Handle installation
	result, err := manager.Install(*forceFlag)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "install_failed",
				Message:     fmt.Sprintf("Installation failed: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(result, format)
	if result.Success {
		os.Exit(ExitSuccess)
	} else {
		os.Exit(result.Error.Code)
	}
}
