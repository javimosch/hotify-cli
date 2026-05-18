package main

import (
	"flag"
	"fmt"
	"os"
)

// TraefikSystemManager handles Traefik installation and management
type TraefikSystemManager struct {
	Target *Remote
}

// NewTraefikSystemManager creates a new Traefik system manager
func NewTraefikSystemManager(target *Remote) *TraefikSystemManager {
	return &TraefikSystemManager{
		Target: target,
	}
}

// CheckStatus checks the current Traefik installation status via HTTP API.
func (t *TraefikSystemManager) CheckStatus() (TraefikStatus, error) {
	client, err := NewDeploymentClient(t.Target)
	if err != nil {
		return TraefikStatus{}, fmt.Errorf("error creating deployment client: %v", err)
	}

	result, err := client.HTTPClient.Get("/api/traefik-system/status")
	if err != nil {
		return TraefikStatus{}, fmt.Errorf("API request failed: %v", err)
	}

	if result["success"] == false {
		return TraefikStatus{}, fmt.Errorf("status check failed: %v", result["error"])
	}

	statusData, ok := result["status"].(map[string]interface{})
	if !ok {
		return TraefikStatus{}, fmt.Errorf("invalid status response")
	}

	return TraefikStatus{
		Installed:      statusData["installed"].(bool),
		Version:        statusData["version"].(string),
		Status:         statusData["status"].(string),
		BinaryPath:     statusData["binary_path"].(string),
		ConfigDir:      statusData["config_dir"].(string),
		ServiceName:    statusData["service_name"].(string),
		SystemdEnabled: statusData["systemd_enabled"].(bool),
	}, nil
}

// Install performs idempotent Traefik installation via HTTP API.
func (t *TraefikSystemManager) Install(force bool) (CommandResult, error) {
	result := CommandResult{
		Version: "1.0",
		Success: false,
		Data:    make(map[string]interface{}),
	}

	client, err := NewDeploymentClient(t.Target)
	if err != nil {
		return result, fmt.Errorf("error creating deployment client: %v", err)
	}

	// Call install API
	payload := map[string]interface{}{"force": force}
	if err := client.HTTPClient.Post("/api/traefik-system/install", payload); err != nil {
		result.Error = &CommandError{
			Code:        ExitTraefikInstallFailed,
			Type:        "traefik_install_failed",
			Message:     fmt.Sprintf("Installation failed: %v", err),
			Recoverable: false,
		}
		return result, nil
	}

	// Get final status
	finalStatus, _ := t.CheckStatus()

	result.Success = true
	result.Data["status"] = finalStatus
	result.Metadata = map[string]interface{}{
		"traefik_version": finalStatus.Version,
		"install_method":  "http_api",
	}

	return result, nil
}

// Remove removes Traefik installation via HTTP API.
func (t *TraefikSystemManager) Remove() (CommandResult, error) {
	result := CommandResult{
		Version: "1.0",
		Success: false,
		Data:    make(map[string]interface{}),
	}

	client, err := NewDeploymentClient(t.Target)
	if err != nil {
		return result, fmt.Errorf("error creating deployment client: %v", err)
	}

	// Call remove API
	if err := client.HTTPClient.Post("/api/traefik-system/remove", nil); err != nil {
		result.Error = &CommandError{
			Code:        ExitTraefikNotInstalled,
			Type:        "traefik_remove_failed",
			Message:     fmt.Sprintf("Removal failed: %v", err),
			Recoverable: false,
		}
		return result, nil
	}

	result.Success = true
	result.Data["note"] = "Configuration directory preserved for safety"

	return result, nil
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
