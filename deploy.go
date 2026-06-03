package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DeploymentClient, HTTPClient, types, validation → see deploy_client.go

// Exit codes for deploy operations
const (
	ExitTraefikNotInstalled     = 90
	ExitTraefikAlreadyInstalled = 91
	ExitTraefikInstallFailed    = 92
	ExitTraefikServiceFailed    = 93
	ExitTraefikConfigInvalid    = 94
	ExitPermissionsError        = 95
	ExitTargetNotFound          = 96
	ExitConnectionTimeout       = 105
)

// handleDeploy handles file transfer: hotify-cli deploy --id <id> --source <path> [--target <name>] [--local] [--setup-dns] [--ip <ip>]
func handleDeploy() {
	deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
	appID := deployCmd.String("id", "", "App ID (required)")
	source := deployCmd.String("source", "", "Source file or directory path (required)")
	target := deployCmd.String("target", "", "Target name (uses default if not specified)")
	local := deployCmd.Bool("local", false, "Execute locally (ignore target)")
	setupDNS := deployCmd.Bool("setup-dns", false, "Also create Cloudflare DNS A record after deploy")
	ip := deployCmd.String("ip", "", "Server IP for DNS (auto-detected if omitted)")

	filteredArgs := filterHumanFlag(os.Args[2:])
	deployCmd.Parse(filteredArgs)
	format := getOutputFormat()

	if *appID == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli deploy --id <id> --source <path>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if *source == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --source",
				Recoverable: false,
				Suggestions: []string{"hotify-cli deploy --id <id> --source ./mybinary"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	var targetObj *Remote
	if !*local {
		var err error
		targetObj, err = getActiveTarget(*target)
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
					Recoverable: false,
					Suggestions: []string{
						"hotify-cli targets --action list",
						"hotify-cli targets --action use --name <name>",
						"hotify-cli deploy --id <id> --source <path> --local",
					},
				},
			}, format)
			os.Exit(ExitTargetNotFound)
		}
	}

	warnings := []string{}

	// Optional DNS setup before file transfer
	if *setupDNS {
		config, cfgErr := loadConfig()
		if cfgErr == nil {
			resolvedIP, warn, resolveErr := resolveServerIP(*ip)
			if resolveErr != nil {
				warnings = append(warnings, fmt.Sprintf("DNS setup skipped: %v", resolveErr))
			} else {
				if warn != "" {
					warnings = append(warnings, warn)
				}
				// Find app domain
				appDomain := *appID + "." + config.Domain
				for _, app := range config.Apps {
					if app.ID == *appID {
						appDomain = app.Domain
						break
					}
				}
				zoneID, zErr := getZoneID(appDomain, config.CloudflareToken, config.AdminEmail)
				if zErr != nil {
					warnings = append(warnings, fmt.Sprintf("DNS setup failed (zone): %v", zErr))
				} else if dnsErr := setupDNSRecord(appDomain, resolvedIP, zoneID, config.CloudflareToken, config.AdminEmail); dnsErr != nil {
					warnings = append(warnings, fmt.Sprintf("DNS setup failed: %v", dnsErr))
				}
			}
		}
	}

	handleDeployAction(*appID, *source, targetObj, *local, format, warnings)
}

func handleDeployAction(appID, source string, target *Remote, isLocal bool, format OutputFormat, warnings []string) {
	if warnings == nil {
		warnings = []string{}
	}
	targetName := "local"
	if target != nil {
		targetName = target.Name
	}
	if format == OutputFormatText {
		fmt.Printf("Deploying app %s to %s\n", appID, targetName)
	}

	// Validate source
	if err := ValidateSource(source); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     fmt.Sprintf("Source validation failed: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check source path exists and is accessible", "Ensure source is a valid file or directory"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Check if source exists
	sourceInfo, err := os.Stat(source)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     fmt.Sprintf("Error accessing source: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check source path is correct", "Ensure file/directory exists"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Determine deployment type and paths
	var deployErr error
	var targetPath string
	var deploymentType string
	defer CleanupTempFiles()

	if isLocal {
		// Local deployment: copy directly on this machine
		localAppsDir := fmt.Sprintf("/tmp/hotify-apps/%s", appID)
		if sourceInfo.IsDir() {
			targetPath = localAppsDir
			deploymentType = "folder"
			if format == OutputFormatText {
				fmt.Printf("Deploying folder locally: %s -> %s\n", source, targetPath)
			}
			deployErr = localCopyDir(source, targetPath)
		} else {
			targetPath = fmt.Sprintf("%s/%s", localAppsDir, filepath.Base(source))
			deploymentType = "binary"
			if format == OutputFormatText {
				fmt.Printf("Deploying binary locally: %s -> %s\n", source, targetPath)
			}
			deployErr = LocalDeploy(source, targetPath)
		}
	} else {
		// Remote deployment via HTTP API
		client, err := NewDeploymentClient(target)
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code:        ExitGenericFailure,
					Type:        "client_error",
					Message:     fmt.Sprintf("Error creating deployment client: %v", err),
					Recoverable: false,
					Suggestions: []string{"Check target configuration", "Verify authentication token"},
				},
			}, format)
			os.Exit(ExitGenericFailure)
		}

		if sourceInfo.IsDir() {
			targetPath = fmt.Sprintf("/tmp/hotify-apps/%s", appID)
			deploymentType = "folder"
			if format == OutputFormatText {
				fmt.Printf("Deploying folder: %s -> %s:%s\n", source, targetName, targetPath)
			}
			deployErr = client.DeployFolder(appID, source, targetPath)
		} else {
			targetPath = fmt.Sprintf("/tmp/hotify-apps/%s/%s", appID, filepath.Base(source))
			deploymentType = "binary"
			if format == OutputFormatText {
				fmt.Printf("Deploying binary: %s -> %s:%s\n", source, targetName, targetPath)
			}
			deployErr = client.DeployBinary(appID, source, targetPath)
		}
	}

	if deployErr != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "deployment_error",
				Message:     fmt.Sprintf("Deployment failed: %v", deployErr),
				Recoverable: true,
				Suggestions: []string{"Check network connectivity to target", "Verify target server is running", "Check target disk space"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":          appID,
			"target":          targetName,
			"deployment_type": deploymentType,
			"source":          source,
			"target_path":     targetPath,
			"local":           isLocal,
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"warnings":  warnings,
		},
	}
	printOutput(result, format)
}

// handleRemoteStart/Stop/Restart/Status → see remote_ops.go
