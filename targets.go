package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// handleTargets handles the targets CLI command
func handleTargets() {
	targetsCmd := flag.NewFlagSet("targets", flag.ExitOnError)
	action := targetsCmd.String("action", "list", "Action: use, list, remove, set-default, validate")
	name := targetsCmd.String("name", "", "Target name")
	
	// Filter out --human flag before parsing
	filteredArgs := filterHumanFlag(os.Args[2:])
	targetsCmd.Parse(filteredArgs)

	// Determine output format (JSON by default, --human for text)
	format := getOutputFormat()

	switch *action {
	case "use":
		handleTargetUse(*name, format)
	case "list":
		handleTargetList(format)
	case "remove":
		handleTargetRemove(*name, format)
	case "set-default":
		handleTargetSetDefault(*name, format)
	case "validate":
		handleTargetValidate(*name, format)
	default:
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "invalid_argument",
				Message:     fmt.Sprintf("Unknown action: %s", *action),
				Recoverable: false,
				Suggestions: []string{"Valid actions: use, list, remove, set-default, validate"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}
}

// handleTargetUse sets a target as the active target for subsequent commands
func handleTargetUse(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli targets --action use --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find target
	var target *Remote
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			target = &config.Remotes[i]
			break
		}
	}

	if target == nil {
		availableTargets := []string{}
		for _, remote := range config.Remotes {
			availableTargets = append(availableTargets, fmt.Sprintf("%s (%s)", remote.Name, remote.URL))
		}
		
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_not_found",
				Message:     fmt.Sprintf("Target '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"Available targets:"},
			},
			Data: map[string]interface{}{
				"available_targets": availableTargets,
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}
	}

func handleTargetList(format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	targets := []map[string]interface{}{}
	for _, remote := range config.Remotes {
		targets = append(targets, map[string]interface{}{
			"name":        remote.Name,
			"url":         remote.URL,
			"ssh_host":    remote.SSHHost,
			"permissions": remote.Permissions,
			"default":     remote.Default,
			"last_used":   remote.LastUsed,
		})
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"targets": targets,
			"count":   len(targets),
		},
	}
	printOutput(result, format)
}

func handleTargetRemove(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli targets --action remove --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find and remove target
	found := false
	var updatedRemotes []Remote
	for _, remote := range config.Remotes {
		if remote.Name == name {
			found = true
		} else {
			updatedRemotes = append(updatedRemotes, remote)
		}
	}

	if !found {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_not_found",
				Message:     fmt.Sprintf("Target '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"List available targets: hotify-cli targets --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	// If we removed the default target, set a new default
	wasDefault := false
	for _, remote := range config.Remotes {
		if remote.Name == name && remote.Default {
			wasDefault = true
			break
		}
	}

	if wasDefault && len(updatedRemotes) > 0 {
		updatedRemotes[0].Default = true
	}

	config.Remotes = updatedRemotes

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_save_failed",
				Message:     fmt.Sprintf("Error saving config: %v", err),
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
			"removed_target": name,
		},
		Metadata: map[string]interface{}{
			"action": "removed",
		},
	}
	printOutput(result, format)
}

func handleTargetSetDefault(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli targets --action set-default --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find target
	var target *Remote
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			target = &config.Remotes[i]
			break
		}
	}

	if target == nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_not_found",
				Message:     fmt.Sprintf("Target '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"List available targets: hotify-cli targets --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	// Set as default
	for i := range config.Remotes {
		config.Remotes[i].Default = (config.Remotes[i].Name == name)
	}

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_save_failed",
				Message:     fmt.Sprintf("Error saving config: %v", err),
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
			"default_target": name,
		},
		Metadata: map[string]interface{}{
			"action": "set_as_default",
		},
	}
	printOutput(result, format)
}

func handleTargetValidate(name string, format OutputFormat) {
	// If no name specified, validate default target
	if name == "" {
		target, err := getActiveTarget("")
		if err != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitTargetNotFound,
					Type:        "no_default_target",
					Message:     fmt.Sprintf("Error: %v", err),
					Recoverable: false,
					Suggestions: []string{
						"Usage: hotify-cli targets --action validate --name <name>",
						"Set default target: hotify-cli targets --action use --name <name>",
					},
				},
			}
			printOutput(result, format)
			os.Exit(ExitTargetNotFound)
		}
		name = target.Name
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find target
	var target *Remote
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			target = &config.Remotes[i]
			break
		}
	}

	if target == nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_not_found",
				Message:     fmt.Sprintf("Target '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"List available targets: hotify-cli targets --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	// Test connection
	security, err := NewSecurityManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "security_manager_failed",
				Message:     fmt.Sprintf("Error creating security manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	token, err := security.DecryptToken(target.AuthToken)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "decryption_failed",
				Message:     fmt.Sprintf("Error decrypting token: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Test connection
	client, err := NewAuthClient(target.URL, token)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "connection_failed",
				Message:     fmt.Sprintf("Error creating auth client: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check URL is correct", "Check network connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	valid, err := client.ValidateToken()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "validation_failed",
				Message:     fmt.Sprintf("Connection test failed for %s: %v", name, err),
				Recoverable: true,
				Suggestions: []string{"Check remote daemon is running", "Verify network connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	if !valid {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "invalid_token",
				Message:     fmt.Sprintf("Connection test failed for %s: invalid token", name),
				Recoverable: false,
				Suggestions: []string{"Re-authenticate: hotify-cli auth --action add --url <url> --token <token> --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	// Update last used
	now := time.Now().Format(time.RFC3339)
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			config.Remotes[i].LastUsed = now
			break
		}
	}
	saveConfig(config)

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"target": map[string]interface{}{
				"name":   name,
				"url":    target.URL,
				"valid":  true,
			},
		},
		Metadata: map[string]interface{}{
			"action": "validated",
		},
	}
	printOutput(result, format)
}

// getDefaultTarget returns the default target from config
func getDefaultTarget() (*Remote, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	for _, remote := range config.Remotes {
		if remote.Default {
			return &remote, nil
		}
	}

	if len(config.Remotes) == 0 {
		return nil, fmt.Errorf("no targets configured")
	}

	return nil, fmt.Errorf("no default target set")
}

// getTargetByName returns a target by name
func getTargetByName(name string) (*Remote, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			return &config.Remotes[i], nil
		}
	}

	return nil, fmt.Errorf("target '%s' not found", name)
}

// resolveTarget resolves a target either by name or default
func resolveTarget(targetName string) (*Remote, error) {
	if targetName != "" {
		return getTargetByName(targetName)
	}
	return getDefaultTarget()
}

// getActiveTarget returns the active target with error handling
func getActiveTarget(targetName string) (*Remote, error) {
	target, err := resolveTarget(targetName)
	if err != nil {
		if targetName == "" {
			return nil, fmt.Errorf("no default target set. Use 'hotify-cli targets --action use --name <name>' to set a default target")
		}
		return nil, fmt.Errorf("target '%s' not found", targetName)
	}

	return target, nil
}

// validateTarget tests if a target is accessible
func validateTarget(target *Remote) error {
	// Test connection to ensure target is accessible
	security, err := NewSecurityManager()
	if err != nil {
		return fmt.Errorf("error creating security manager: %v", err)
	}

	// Decrypt token
	token, err := security.DecryptToken(target.AuthToken)
	if err != nil {
		return fmt.Errorf("error decrypting token: %v", err)
	}

	// Create auth client and test connection
	client, err := NewAuthClient(target.URL, token)
	if err != nil {
		return fmt.Errorf("error creating auth client: %v", err)
	}

	if valid, err := client.ValidateToken(); err != nil || !valid {
		return fmt.Errorf("target '%s' is not accessible: %v", target.Name, err)
	}

	return nil
}