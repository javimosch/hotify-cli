package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// setupRoutingForApp regenerates only the Traefik dynamic config (router + service)
// for all apps, preserving foreign sections. It does not touch traefik.yml or ACME
// settings, and it only restarts Traefik when restart is true.
func setupRoutingForApp(appID string, restart bool) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	found := false
	for _, app := range config.Apps {
		if app.ID == appID {
			found = true
			if err := validateApp(app); err != nil {
				return err
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("app '%s' not found in configuration", appID)
	}

	// Regenerate the dynamic config and carry through anything hotify did not author.
	if err := writeDynamicConfigAtomic(config); err != nil {
		return fmt.Errorf("error writing dynamic config: %v", err)
	}

	if restart {
		if err := restartTraefik(); err != nil {
			return fmt.Errorf("error restarting traefik: %v", err)
		}
	}

	return nil
}

// handleSetupRoutingCLI handles: hotify-cli setup-routing --id <id> [--restart] [--dry-run] [--target <name>] [--local]
func handleSetupRoutingCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-routing", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	restart := cmd.Bool("restart", false, "Restart Traefik after regenerating routing")
	dryRun := cmd.Bool("dry-run", false, "Preview changes without applying (diff)")
	target := cmd.String("target", "", "Target name for remote execution")
	local := cmd.Bool("local", false, "Execute directly on local server")
	cmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli setup-routing --id <app-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if *local {
		if *dryRun {
			config, err := loadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(ExitGenericFailure)
			}

			proposed, err := buildDynamicYAML(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error building dynamic config: %v\n", err)
				os.Exit(ExitGenericFailure)
			}
			// Preserve foreign sections just like the real writer does.
			proposed, _ = mergeForeignSections(proposed, dynamicTargetPath(config), config)

			current := ""
			if data, err := os.ReadFile(dynamicTargetPath(config)); err == nil {
				current = string(data)
			}

			diff := simpleDiff(current, proposed)
			if diff == "" {
				fmt.Println("No changes — current dynamic config matches proposed config.")
			} else {
				fmt.Println("📋 Proposed routing changes (--dry-run):")
				fmt.Println()
				for _, line := range strings.Split(diff, "\n") {
					if len(line) == 0 {
						continue
					}
					prefix := line[0]
					content := line[1:]
					switch prefix {
					case '+':
						fmt.Printf("  \033[32m+ %s\033[0m\n", content)
					case '-':
						fmt.Printf("  \033[31m- %s\033[0m\n", content)
					default:
						fmt.Printf("    %s\n", content)
					}
				}
			}
			fmt.Println()
			fmt.Println("Dry run — no changes were made.")
			return
		}

		if err := setupRoutingForApp(*id, *restart); err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitTraefikConfigInvalid, Type: "traefik_error",
					Message:     err.Error(),
					Recoverable: true,
					Suggestions: []string{
						"Check app exists: hotify-cli list",
						"Check Traefik is installed: hotify-cli traefik-system --status",
					},
				},
			}, format)
			os.Exit(ExitTraefikConfigInvalid)
		}

		config, _ := loadConfig()
		var backendURL, pathPrefix string
		if config != nil {
			for _, app := range config.Apps {
				if app.ID == *id {
					backendURL = app.BackendURL
					pathPrefix = app.PathPrefix
					break
				}
			}
		}

		printOutput(CommandResult{
			Version: Version,
			Success: true,
			Data: map[string]interface{}{
				"app_id":      *id,
				"restarted":   *restart,
				"backend_url": backendURL,
				"path_prefix": pathPrefix,
				"action":      "routing_configured",
			},
		}, format)
		return
	}

	// Remote mode: use HTTP API
	targetObj, err := getActiveTarget(*target)
	if err != nil {
		exitTargetNotFound(format, err)
	}

	client, err := NewDeploymentClient(targetObj)
	if err != nil {
		exitClientError(format, err)
	}

	result, err := client.SetupRoutingApp(*id, *restart, *dryRun)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "remote_error",
				Message:     fmt.Sprintf("Failed to setup routing remotely: %v", err),
				Recoverable: true,
				Suggestions: []string{
					"Check target connectivity",
					"Verify hotify daemon is running on remote",
					"Check app exists on remote",
				},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data:    result,
		Metadata: map[string]interface{}{
			"target": targetObj.Name,
		},
	}, format)
}
