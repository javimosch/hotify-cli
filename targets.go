package main

import (
	"flag"
	"fmt"
	"os"
)

// handleTargets handles the targets CLI command
func handleTargets() {
	targetsCmd := flag.NewFlagSet("targets", flag.ExitOnError)
	action := targetsCmd.String("action", "list", "Action: use, list, remove, set-default, validate")
	name := targetsCmd.String("name", "", "Target name")
	targetsCmd.Parse(os.Args[2:])

	switch *action {
	case "use":
		handleTargetUse(*name)
	case "list":
		handleTargetList()
	case "remove":
		handleTargetRemove(*name)
	case "set-default":
		handleTargetSetDefault(*name)
	case "validate":
		handleTargetValidate(*name)
	default:
		fmt.Println("Unknown action:", *action)
		fmt.Println("Valid actions: use, list, remove, set-default, validate")
		os.Exit(1)
	}
}

// handleTargetUse sets a target as the active target for subsequent commands
func handleTargetUse(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli targets --action use --name <name>")
		os.Exit(1)
	}

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
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
		fmt.Printf("Target '%s' not found\n", name)
		fmt.Println("Available targets:")
		for _, remote := range config.Remotes {
			fmt.Printf("  - %s (%s)\n", remote.Name, remote.URL)
		}
		os.Exit(1)
	}

	// Set as default
	for i := range config.Remotes {
		config.Remotes[i].Default = (config.Remotes[i].Name == name)
	}

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Target set to: %s\n", name)
	fmt.Printf("URL: %s\n", target.URL)
	fmt.Printf("Permissions: %v\n", target.Permissions)
}

// handleTargetList lists all configured targets
func handleTargetList() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Remotes) == 0 {
		fmt.Println("No targets configured")
		fmt.Println("Add a target with: hotify-cli auth --action add --url <url> --token <token> --name <name>")
		return
	}

	fmt.Println("Configured Targets:")
	fmt.Println("===================")
	for _, remote := range config.Remotes {
		defaultMark := ""
		if remote.Default {
			defaultMark = " (default)"
		}
		fmt.Printf("Name: %s%s\n", remote.Name, defaultMark)
		fmt.Printf("URL: %s\n", remote.URL)
		fmt.Printf("Permissions: %v\n", remote.Permissions)
		fmt.Printf("Last Used: %s\n", remote.LastUsed)
		fmt.Println()
	}
}

// handleTargetRemove removes a target
func handleTargetRemove(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli targets --action remove --name <name>")
		os.Exit(1)
	}

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
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
		fmt.Printf("Target '%s' not found\n", name)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Target removed: %s\n", name)
	if wasDefault && len(updatedRemotes) > 0 {
		fmt.Printf("New default target: %s\n", updatedRemotes[0].Name)
	}
}

// handleTargetSetDefault sets a target as default (alias for use)
func handleTargetSetDefault(name string) {
	handleTargetUse(name)
}

// handleTargetValidate validates a target's connectivity
func handleTargetValidate(name string) {
	target, err := getActiveTarget(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := validateTarget(target); err != nil {
		fmt.Fprintf(os.Stderr, "Validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Target '%s' is accessible and authenticated\n", target.Name)
	fmt.Printf("URL: %s\n", target.URL)
	fmt.Printf("Permissions: %v\n", target.Permissions)
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

	for _, remote := range config.Remotes {
		if remote.Name == name {
			return &remote, nil
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
