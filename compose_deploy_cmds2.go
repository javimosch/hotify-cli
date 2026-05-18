package main

// compose-copy-dir, volume-init, and setup-compose commands.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// handleComposeCopyDirCLI implements `hotify-cli compose-copy-dir`.
// Copies a local directory into the remote compose_path for the app.
func handleComposeCopyDirCLI() {
	cmd := flag.NewFlagSet("compose-copy-dir", flag.ExitOnError)
	appID := cmd.String("id", "", "App ID (required)")
	dirName := cmd.String("dir", "", "Subdirectory name on remote, relative to compose_path (required)")
	source := cmd.String("source", "", "Local directory to copy (required)")
	targetName := cmd.String("target", "", "Target name (uses default if not specified)")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" || *dirName == "" || *source == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flags: --id, --dir, and --source",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli compose-copy-dir --id <id> --dir webui --source /local/path/webui",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		exitConfigError(format, err)
	}
	app := findApp(config, *appID)
	if app == nil {
		exitAppNotFound(format, *appID)
	}
	if app.ComposePath == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "App has no compose_path configured. Set it with: hotify-cli setup --id " + *appID + " --compose-path /remote/path",
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if _, statErr := os.Stat(*source); statErr != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Source directory not found: %v", statErr),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	target, err := getActiveTarget(*targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}

	remoteDest := filepath.Join(app.ComposePath, *dirName)
	if format == OutputFormatText {
		fmt.Printf("Copying %s → %s:%s\n", *source, target.Name, remoteDest)
	}

	if err := client.DeployFolder(*appID, *source, remoteDest); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "copy_error",
				Message:     fmt.Sprintf("Failed to copy directory: %v", err),
				Recoverable: true,
				Suggestions: []string{
					"Check target connectivity",
					"Verify hotify daemon is running on remote",
				},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{
			"app_id":      *appID,
			"target":      target.Name,
			"dir":         *dirName,
			"source":      *source,
			"remote_path": remoteDest,
		},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

// handleVolumeInitCLI implements `hotify-cli volume-init`.
// Sends a local directory to the remote daemon so it can populate a Docker
// named volume (requires write access to /var/lib/docker/volumes/ on remote).
func handleVolumeInitCLI() {
	cmd := flag.NewFlagSet("volume-init", flag.ExitOnError)
	appID := cmd.String("id", "", "App ID (required)")
	volumeName := cmd.String("volume", "", "Docker volume name (e.g. cir-webui → volume <id>_cir-webui) (required)")
	source := cmd.String("source", "", "Local directory whose contents populate the volume (required)")
	targetName := cmd.String("target", "", "Target name (uses default if not specified)")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" || *volumeName == "" || *source == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flags: --id, --volume, and --source",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli volume-init --id cir-doc-gen --volume cir-webui --source /local/webui",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if _, statErr := os.Stat(*source); statErr != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Source directory not found: %v", statErr),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	target, err := getActiveTarget(*targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}

	if format == OutputFormatText {
		fmt.Printf("Initializing volume %s_%s on %s from %s\n", *appID, *volumeName, target.Name, *source)
		fmt.Println("NOTE: Remote daemon needs write access to /var/lib/docker/volumes/")
	}

	if err := client.VolumeInit(*appID, *volumeName, *source); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "volume_init_error",
				Message:     fmt.Sprintf("Volume init failed: %v", err),
				Recoverable: true,
				Suggestions: []string{
					"Ensure hotify daemon has write access to /var/lib/docker/volumes/",
					"Configure Docker volume permissions on remote",
				},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{
			"app_id":  *appID,
			"target":  target.Name,
			"volume":  fmt.Sprintf("%s_%s", *appID, *volumeName),
			"source":  *source,
		},
		Metadata: map[string]interface{}{
			"warning":   "Volume init requires write access to /var/lib/docker/volumes/ on the remote",
			"timestamp": time.Now().Unix(),
		},
	}, format)
}

// handleSetupComposeCLI implements `hotify-cli setup-compose`.
// Combines app registration + file deployment in one command.
func handleSetupComposeCLI() {
	cmd := flag.NewFlagSet("setup-compose", flag.ExitOnError)
	appID := cmd.String("id", "", "App ID (required)")
	name := cmd.String("name", "", "App display name (required for new apps)")
	domain := cmd.String("domain", "", "App subdomain (required for new apps)")
	port := cmd.Int("port", 0, "App port (required for new apps)")
	startCmd := cmd.String("cmd", "", "Command to start app (required for new apps)")
	source := cmd.String("source", "", "Local project directory to deploy (required)")
	composeFile := cmd.String("compose-file", "", "Compose file name (e.g. docker-compose.yml)")
	remotePath := cmd.String("remote-path", "", "Remote destination path (default: /home/dk1/<id>)")
	setupDNS := cmd.Bool("setup-dns", false, "Also create Cloudflare DNS A record")
	ip := cmd.String("ip", "", "Server IP for DNS (auto-detected if omitted)")
	startAfter := cmd.Bool("start", false, "Start the service after deploying")
	targetName := cmd.String("target", "", "Target name (uses default if not specified)")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" || *source == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flags: --id and --source",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli setup-compose --id myapp --name 'My App' --domain myapp --port 8080 --cmd 'docker compose up -d' --source /path/to/project --compose-file docker-compose.yml",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		exitConfigError(format, err)
	}

	// Resolve paths
	destPath := *remotePath
	if destPath == "" {
		destPath = fmt.Sprintf("/home/dk1/%s", *appID)
	}
	cfName := *composeFile
	if cfName == "" {
		cfName = autoDetectComposeFile(*source)
	}

	// Upsert app config
	existingIdx := -1
	for i, app := range config.Apps {
		if app.ID == *appID {
			existingIdx = i
			break
		}
	}

	if existingIdx >= 0 {
		app := config.Apps[existingIdx]
		if *name != "" {
			app.Name = *name
		}
		if *domain != "" {
			app.Domain = fmt.Sprintf("%s.%s", *domain, config.Domain)
		}
		if *port != 0 {
			app.Port = *port
		}
		if *startCmd != "" {
			app.Command = *startCmd
		}
		app.ComposePath = destPath
		if cfName != "" {
			app.ComposeFile = cfName
		}
		config.Apps[existingIdx] = app
	} else {
		if *name == "" || *domain == "" || *port == 0 || *startCmd == "" {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitInvalidArgument, Type: "validation_error",
					Message:     "New app requires --name, --domain, --port, and --cmd",
					Recoverable: false,
					Suggestions: []string{
						"hotify-cli setup-compose --id myapp --name 'My App' --domain myapp --port 8080 --cmd 'docker compose up -d' --source /path",
					},
				},
			}, format)
			os.Exit(ExitInvalidArgument)
		}
		config.Apps = append(config.Apps, App{
			ID:          *appID,
			Name:        *name,
			Domain:      fmt.Sprintf("%s.%s", *domain, config.Domain),
			Port:        *port,
			Command:     *startCmd,
			Status:      "stopped",
			ComposePath: destPath,
			ComposeFile: cfName,
		})
		existingIdx = len(config.Apps) - 1
	}

	if err := saveConfig(config); err != nil {
		exitConfigError(format, err)
	}

	warnings := []string{}

	// Optional DNS setup
	if *setupDNS {
		app := config.Apps[existingIdx]
		resolvedIP, warn, resolveErr := resolveServerIP(*ip)
		if resolveErr != nil {
			warnings = append(warnings, fmt.Sprintf("DNS setup skipped: %v", resolveErr))
		} else {
			if warn != "" {
				warnings = append(warnings, warn)
			}
			zoneID, zErr := getZoneID(app.Domain, config.CloudflareToken, config.AdminEmail)
			if zErr != nil {
				warnings = append(warnings, fmt.Sprintf("DNS setup failed: %v", zErr))
			} else if dnsErr := setupDNSRecord(app.Domain, resolvedIP, zoneID, config.CloudflareToken, config.AdminEmail); dnsErr != nil {
				warnings = append(warnings, fmt.Sprintf("DNS record error: %v", dnsErr))
			}
		}
	}

	// Deploy project files
	target, err := getActiveTarget(*targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}

	if _, statErr := os.Stat(*source); statErr != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Source directory not found: %v", statErr),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if format == OutputFormatText {
		fmt.Printf("Deploying project %s → %s:%s\n", *source, target.Name, destPath)
	}

	if err := client.DeployFolder(*appID, *source, destPath); err != nil {
		warnings = append(warnings, fmt.Sprintf("File deploy failed: %v", err))
	}

	if *startAfter {
		if err := client.StartApp(*appID); err != nil {
			warnings = append(warnings, fmt.Sprintf("Start failed: %v", err))
		}
	}

	app := config.Apps[existingIdx]
	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{
			"id":           app.ID,
			"name":         app.Name,
			"domain":       app.Domain,
			"port":         app.Port,
			"compose_file": cfName,
			"compose_path": destPath,
			"target":       target.Name,
		},
		Metadata: map[string]interface{}{"warnings": warnings, "timestamp": time.Now().Unix()},
	}, format)
}
