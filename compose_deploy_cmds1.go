package main

// deploy-compose and compose-sync commands.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// handleDeployCompose implements `hotify-cli deploy-compose`.
// Copies the full project tree to the remote compose_path via the HTTP API.
// It also copies .env and updates the app config with the resolved paths.
func handleDeployCompose() {
	cmd := flag.NewFlagSet("deploy-compose", flag.ExitOnError)
	appID := cmd.String("id", "", "App ID (required)")
	source := cmd.String("source", "", "Local project directory containing compose file and assets (required)")
	composeFile := cmd.String("compose-file", "", "Compose file name inside source (default: auto-detect)")
	remotePath := cmd.String("remote-path", "", "Override remote destination path (default: app's compose_path or /tmp/hotify-apps/<id>)")
	targetName := cmd.String("target", "", "Target name (uses default if not specified)")
	local := cmd.Bool("local", false, "Execute locally (ignore target)")
	startAfter := cmd.Bool("start", false, "Run start command after copying files")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" || *source == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flags: --id and --source",
				Recoverable: false,
				Suggestions: []string{"hotify-cli deploy-compose --id <id> --source /path/to/project"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if _, err := os.Stat(*source); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Source directory not found: %v", err),
				Recoverable: false,
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

	// Resolve remote path: flag > app.ComposePath > default
	destPath := *remotePath
	if destPath == "" {
		destPath = app.ComposePath
	}
	if destPath == "" {
		destPath = fmt.Sprintf("/tmp/hotify-apps/%s", *appID)
	}

	// Resolve compose file name
	cfName := *composeFile
	if cfName == "" {
		cfName = app.ComposeFile
	}
	if cfName == "" {
		cfName = autoDetectComposeFile(*source)
	}

	warnings := []string{}
	copiedItems := []string{}
	displayTarget := "local"

	var copyErr error
	if *local {
		// Local: copy using filesystem
		if format == OutputFormatText {
			fmt.Printf("Copying project %s → %s (local)\n", *source, destPath)
		}
		copyErr = localCopyDir(*source, destPath)
	} else {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		displayTarget = target.Name
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if format == OutputFormatText {
			fmt.Printf("Copying project %s → %s:%s\n", *source, target.Name, destPath)
		}
		copyErr = client.DeployFolder(*appID, *source, destPath)
	}

	if copyErr != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "deploy_error",
				Message:     fmt.Sprintf("Failed to copy project tree: %v", copyErr),
				Recoverable: true,
				Suggestions: []string{
					"Check target connectivity",
					"Verify hotify daemon is running on remote",
				},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}
	copiedItems = append(copiedItems, "project tree")

	// Update app config with resolved paths if they changed
	changed := false
	if app.ComposePath != destPath {
		app.ComposePath = destPath
		changed = true
	}
	if cfName != "" && app.ComposeFile != cfName {
		app.ComposeFile = cfName
		changed = true
	}
	if changed {
		for i := range config.Apps {
			if config.Apps[i].ID == *appID {
				config.Apps[i] = *app
				break
			}
		}
		if saveErr := saveConfig(config); saveErr != nil {
			warnings = append(warnings, fmt.Sprintf("Config save warning: %v", saveErr))
		}
	}

	// Optionally start service (remote only)
	if *startAfter && !*local {
		target, _ := getActiveTarget(*targetName)
		if target != nil {
			client, _ := NewDeploymentClient(target)
			if client != nil {
				if format == OutputFormatText {
					fmt.Printf("Starting service for %s...\n", *appID)
				}
				if err := client.StartApp(*appID); err != nil {
					warnings = append(warnings, fmt.Sprintf("Start failed (files deployed OK): %v", err))
				} else {
					copiedItems = append(copiedItems, "service started")
				}
			}
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{
			"app_id":       *appID,
			"target":       displayTarget,
			"remote_path":  destPath,
			"compose_file": cfName,
			"copied":       copiedItems,
			"local":        *local,
		},
		Metadata: map[string]interface{}{"warnings": warnings, "timestamp": time.Now().Unix()},
	}, format)
}

// handleComposeSyncCLI implements `hotify-cli compose-sync`.
// Syncs the compose file (and optionally .env) from a local directory to the
// remote compose_path without replacing the full project tree.
func handleComposeSyncCLI() {
	cmd := flag.NewFlagSet("compose-sync", flag.ExitOnError)
	appID := cmd.String("id", "", "App ID (required)")
	source := cmd.String("source", "", "Local directory containing compose file (default: current working directory)")
	withEnv := cmd.Bool("env", true, "Also sync .env file if present")
	restart := cmd.Bool("restart", false, "Restart the service after sync")
	targetName := cmd.String("target", "", "Target name (uses default if not specified)")
	local := cmd.Bool("local", false, "Execute locally (ignore target)")
	cmd.Parse(filterHumanFlag(os.Args[2:]))
	format := getOutputFormat()

	if *appID == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli compose-sync --id <id>"},
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

	srcDir := *source
	if srcDir == "" {
		srcDir, _ = os.Getwd()
	}

	cfName := app.ComposeFile
	if cfName == "" {
		cfName = autoDetectComposeFile(srcDir)
	}
	if cfName == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "No compose file found. Set it with: hotify-cli setup --id " + *appID + " --compose-file <filename>",
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	warnings := []string{}
	synced := []string{}
	displayTarget := "local"

	var client *DeploymentClient
	if !*local {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		displayTarget = target.Name
		client, err = NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
	}

	// Sync compose file
	localCompose := filepath.Join(srcDir, cfName)
	if _, statErr := os.Stat(localCompose); statErr == nil {
		remoteDest := filepath.Join(app.ComposePath, cfName)
		if *local {
			if format == OutputFormatText {
				fmt.Printf("Syncing %s → %s (local)\n", cfName, remoteDest)
			}
			if err := copyFile(localCompose, remoteDest, 0644); err != nil {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{Code: ExitGenericFailure, Type: "sync_error", Message: fmt.Sprintf("Failed to sync compose file: %v", err), Recoverable: true},
				}, format)
				os.Exit(ExitGenericFailure)
			}
		} else {
			if format == OutputFormatText {
				fmt.Printf("Syncing %s → %s:%s\n", cfName, displayTarget, remoteDest)
			}
			if err := client.DeployBinary(*appID, localCompose, remoteDest); err != nil {
				printOutput(CommandResult{
					Version: Version, Success: false,
					Error: &CommandError{Code: ExitGenericFailure, Type: "sync_error", Message: fmt.Sprintf("Failed to sync compose file: %v", err), Recoverable: true},
				}, format)
				os.Exit(ExitGenericFailure)
			}
		}
		synced = append(synced, cfName)
	} else {
		warnings = append(warnings, fmt.Sprintf("Compose file not found locally: %s", localCompose))
	}

	// Sync .env if present and requested
	if *withEnv {
		localEnv := filepath.Join(srcDir, ".env")
		if _, statErr := os.Stat(localEnv); statErr == nil {
			remoteEnv := filepath.Join(app.ComposePath, ".env")
			if *local {
				if err := copyFile(localEnv, remoteEnv, 0600); err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to sync .env: %v", err))
				} else {
					synced = append(synced, ".env")
				}
			} else {
				if format == OutputFormatText {
					fmt.Printf("Syncing .env → %s:%s\n", displayTarget, remoteEnv)
				}
				if err := client.DeployBinary(*appID, localEnv, remoteEnv); err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to sync .env: %v", err))
				} else {
					synced = append(synced, ".env")
				}
			}
		}
	}

	// Optionally restart (remote only)
	if *restart && !*local && client != nil {
		if format == OutputFormatText {
			fmt.Printf("Restarting service for %s...\n", *appID)
		}
		if err := client.RestartApp(*appID); err != nil {
			warnings = append(warnings, fmt.Sprintf("Restart failed (sync OK): %v", err))
		} else {
			synced = append(synced, "service restarted")
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{
			"app_id": *appID,
			"target": displayTarget,
			"synced": synced,
			"local":  *local,
		},
		Metadata: map[string]interface{}{"warnings": warnings, "timestamp": time.Now().Unix()},
	}, format)
}
