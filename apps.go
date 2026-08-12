package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// getFullDomain returns the full domain, avoiding double base domain suffix.
// If the domain already ends with the base domain (e.g. "gob.dk1.intrane.fr"
// when base is "intrane.fr"), it is returned as-is.  Otherwise the base is
// appended, so "gob.dk1" becomes "gob.dk1.intrane.fr".
func getFullDomain(domain, baseDomain string) string {
	// If domain already ends with the base domain, use it as-is
	if strings.HasSuffix(domain, "."+baseDomain) {
		return domain
	}
	// Otherwise, append base domain
	return fmt.Sprintf("%s.%s", domain, baseDomain)
}

// setupApp handles both "add" (new) and "setup" (upsert) logic.
// isUpsert=false enforces unique ID (add), isUpsert=true allows update.
func setupApp(isUpsert bool) {
	format := getOutputFormat()

	cmd := flag.NewFlagSet("setup", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	name := cmd.String("name", "", "App display name")
	domain := cmd.String("domain", "", "App subdomain")
	port := cmd.Int("port", 0, "App port")
	command := cmd.String("cmd", "", "Command to start app")
	source := cmd.String("source", "", "App source URL or repo (optional metadata)")
	setupDNS := cmd.Bool("setup-dns", false, "Also create Cloudflare DNS A record after saving")
	ip := cmd.String("ip", "", "Server IP for DNS (auto-detected if omitted)")
	composeFile := cmd.String("compose-file", "", "Docker Compose file to use (e.g. compose.binary.yml)")
	composePath := cmd.String("compose-path", "", "Path on remote where compose files live")
	backendURL := cmd.String("backend-url", "", "Custom backend URL for external reverse proxy (e.g. http://100.114.4.57:8080)")
	pathPrefix := cmd.String("path-prefix", "", "Path prefix for Traefik middleware (e.g., /slv2 for sl-cli sites)")
	targetName := cmd.String("target", "", "Target name for remote execution")
	local := cmd.Bool("local", false, "Execute locally (ignore target)")
	cmd.Parse(filterHumanFlag(os.Args[2:]))

	// Remote mode: forward to remote daemon
	if !*local && *targetName != "" {
		handleSetupAppRemote(*id, *name, *domain, *port, *command, *source, *composeFile, *composePath, *backendURL, *pathPrefix, *setupDNS, *ip, *targetName, format)
		return
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	if *id == "" {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli setup --id <id> --domain <subdomain> --port <port> --cmd <command>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Find existing app
	existingIdx := -1
	for i, app := range config.Apps {
		if app.ID == *id {
			existingIdx = i
			break
		}
	}

	if !isUpsert && existingIdx >= 0 {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "duplicate_id",
				Message:     fmt.Sprintf("App with ID '%s' already exists. Use 'hotify-cli setup' to update it.", *id),
				Recoverable: false,
				Suggestions: []string{"hotify-cli setup --id " + *id + " --port <new-port>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	if existingIdx >= 0 {
		// Update existing app fields selectively
		app := config.Apps[existingIdx]
		if *name != "" {
			app.Name = *name
		}
		if *domain != "" {
			app.Domain = getFullDomain(*domain, config.Domain)
		}
		if *port != 0 {
			app.Port = *port
		}
		if *command != "" {
			app.Command = *command
		}
		if *source != "" {
			app.Source = *source
		}
		if *composeFile != "" {
			app.ComposeFile = *composeFile
		}
		if *composePath != "" {
			app.ComposePath = *composePath
		}
		if *backendURL != "" {
			app.BackendURL = *backendURL
		}
		if *pathPrefix != "" {
			app.PathPrefix = *pathPrefix
		}
		config.Apps[existingIdx] = app
	} else {
		// New app — all required fields must be present
		if *name == "" || *domain == "" || *port == 0 || (*command == "" && *backendURL == "") {
			result := CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitInvalidArgument, Type: "validation_error",
					Message:     "New app requires --name, --domain, and --port; --cmd is optional when --backend-url is set",
					Recoverable: false,
					Suggestions: []string{"hotify-cli setup --id <id> --name <name> --domain <subdomain> --port <port> --backend-url <url>", "hotify-cli setup --id <id> --name <name> --domain <subdomain> --port <port> --cmd <command>"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
		}
		config.Apps = append(config.Apps, App{
			ID:          *id,
			Name:        *name,
			Domain:      getFullDomain(*domain, config.Domain),
			Port:        *port,
			Command:     *command,
			Source:      *source,
			Status:      "stopped",
			ComposeFile: *composeFile,
			ComposePath: *composePath,
			BackendURL:  *backendURL,
			PathPrefix:  *pathPrefix,
		})
		existingIdx = len(config.Apps) - 1
	}

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	app := config.Apps[existingIdx]
	warnings := []string{}

	// Optional DNS setup
	if *setupDNS {
		resolvedIP, warn, resolveErr := resolveServerIP(*ip)
		if resolveErr != nil {
			warnings = append(warnings, fmt.Sprintf("DNS setup skipped: could not determine server IP: %v", resolveErr))
		} else {
			if warn != "" {
				warnings = append(warnings, warn)
			}
			zoneID, zErr := getZoneID(app.Domain, config.CloudflareToken, config.AdminEmail)
			if zErr != nil {
				warnings = append(warnings, fmt.Sprintf("DNS setup failed (zone lookup): %v", zErr))
			} else if dnsErr := setupDNSRecord(app.Domain, resolvedIP, zoneID, config.CloudflareToken, config.AdminEmail); dnsErr != nil {
				warnings = append(warnings, fmt.Sprintf("DNS setup failed: %v", dnsErr))
			}
		}
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"id":           app.ID,
			"name":         app.Name,
			"domain":       app.Domain,
			"port":         app.Port,
			"cmd":          app.Command,
			"compose_file": app.ComposeFile,
			"compose_path": app.ComposePath,
			"backend_url":  app.BackendURL,
			"path_prefix":  app.PathPrefix,
		},
		Metadata: map[string]interface{}{
			"warnings": warnings,
		},
	}
	printOutput(result, format)
}

// addApp is the legacy "add" entrypoint (enforces unique ID)
func addApp() { setupApp(false) }

// editApp is kept for backward compatibility, delegates to upsert
func editApp() { setupApp(true) }

func removeApp() {
	format := getOutputFormat()
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
	id := removeCmd.String("id", "", "App ID (required)")
	targetName := removeCmd.String("target", "", "Target name for remote execution")
	local := removeCmd.Bool("local", false, "Execute locally (ignore target)")
	removeCmd.Parse(filterHumanFlag(os.Args[2:]))

	// Remote mode
	if !*local && *targetName != "" {
		handleRemoveAppRemote(*id, *targetName, format)
		return
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	if *id == "" {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli remove --id <id>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	found := false
	var updatedApps []App
	for _, app := range config.Apps {
		if app.ID != *id {
			updatedApps = append(updatedApps, app)
		} else {
			found = true
		}
	}

	if !found {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "not_found",
				Message:     fmt.Sprintf("App with ID '%s' not found", *id),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config.Apps = updatedApps
	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data:    map[string]interface{}{"id": *id, "action": "removed"},
		Metadata: map[string]interface{}{
			"warnings": []string{
				"DNS record for this app was NOT removed from Cloudflare",
				"Traefik routing config was NOT cleaned up",
				"Run 'hotify-cli prune --id " + *id + "' to remove DNS and Traefik config",
			},
		},
	}
	printOutput(result, format)
}

func listApps() {
	format := getOutputFormat()
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	targetName := listCmd.String("target", "", "Target name for remote execution")
	local := listCmd.Bool("local", false, "Execute locally (ignore target)")
	listCmd.Parse(filterHumanFlag(os.Args[2:]))

	// Remote mode
	if !*local && *targetName != "" {
		handleListAppsRemote(*targetName, format)
		return
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	apps := make([]map[string]interface{}, 0, len(config.Apps))
	for _, app := range config.Apps {
		apps = append(apps, map[string]interface{}{
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

	if format == OutputFormatText {
		if len(apps) == 0 {
			fmt.Println("No apps configured")
			return
		}
		fmt.Println("Configured Apps:")
		fmt.Println("================")
		for _, app := range config.Apps {
			fmt.Printf("ID: %s\n", app.ID)
			fmt.Printf("  Name:   %s\n", app.Name)
			fmt.Printf("  Domain: %s\n", app.Domain)
			fmt.Printf("  Port:   %d\n", app.Port)
			fmt.Printf("  Cmd:    %s\n", app.Command)
			if app.Source != "" {
				fmt.Printf("  Source: %s\n", app.Source)
			}
			if app.ComposeFile != "" {
				fmt.Printf("  Compose: %s (at %s)\n", app.ComposeFile, app.ComposePath)
			}
			if app.BackendURL != "" {
				fmt.Printf("  Backend: %s\n", app.BackendURL)
			}
			if app.PathPrefix != "" {
				fmt.Printf("  Path prefix: %s\n", app.PathPrefix)
			}
			fmt.Printf("  Status: %s\n", app.Status)
			fmt.Println()
		}
		return
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data:    map[string]interface{}{"apps": apps, "count": len(apps)},
	}
	printOutput(result, format)
}

// ─── Remote helpers ───────────────────────────────────────────────────────────

func handleSetupAppRemote(appID, name, domain string, port int, command, src, composeFile, composePath, backendURL, pathPrefix string, setupDNS bool, ip, targetName string, format OutputFormat) {
	target, err := getActiveTarget(targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}
	payload := map[string]interface{}{
		"name": name, "domain": domain, "port": port, "cmd": command,
		"source": src, "compose_file": composeFile, "compose_path": composePath,
		"backend_url": backendURL, "path_prefix": pathPrefix, "setup_dns": setupDNS, "ip": ip,
	}
	result, err := client.SetupAppConfig(appID, payload)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true},
		}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:     result,
		Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()},
	}, format)
}

func handleRemoveAppRemote(appID, targetName string, format OutputFormat) {
	target, err := getActiveTarget(targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}
	result, err := client.RemoveAppConfig(appID)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true},
		}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:     result,
		Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()},
	}, format)
}

func handleListAppsRemote(targetName string, format OutputFormat) {
	target, err := getActiveTarget(targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}
	result, err := client.ListAppsRemote()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true},
		}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:     result,
		Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()},
	}, format)
}
