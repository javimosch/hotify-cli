package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// handleSetupDNSCLI handles: hotify-cli setup-dns --id <id> [--ip <ip>] [--target <name>] [--local]
func handleSetupDNSCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-dns", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	ip := cmd.String("ip", "", "Server IP (auto-detected if omitted)")
	target := cmd.String("target", "", "Target name (uses default if not specified)")
	local := cmd.Bool("local", false, "Execute directly on local server")
	cmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli setup-dns --id <app-id> [--ip <ip>]"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if *local {
		// Local mode: execute directly on local server
		resolvedIP, warn, err := resolveServerIP(*ip)
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitGenericFailure, Type: "ip_resolution_error",
					Message:     fmt.Sprintf("Could not determine server IP: %v", err),
					Recoverable: true,
					Suggestions: []string{"hotify-cli setup-dns --id <id> --ip <your-public-ip>"},
				},
			}, format)
			os.Exit(ExitGenericFailure)
		}

		warnings := []string{}
		if warn != "" {
			warnings = append(warnings, warn)
		}

		if err := setupDNSForApp(*id, resolvedIP); err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitGenericFailure, Type: "dns_error",
					Message:     err.Error(),
					Recoverable: true,
					Suggestions: []string{"Check Cloudflare token permissions", "Verify domain is managed by Cloudflare"},
				},
			}, format)
			os.Exit(ExitGenericFailure)
		}

		printOutput(CommandResult{
			Version: Version,
			Success: true,
			Data: map[string]interface{}{
				"app_id": *id,
				"ip":     resolvedIP,
				"action": "dns_configured",
			},
			Metadata: map[string]interface{}{"warnings": warnings},
		}, format)

		// Cross-suggest: if Traefik is not configured, suggest it
		suggestMissingTraefikSetup(*id)

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

	if err := client.SetupDNSApp(*id, *ip); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "remote_error",
				Message:     fmt.Sprintf("Failed to setup DNS remotely: %v", err),
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
		Data: map[string]interface{}{
			"app_id": *id,
			"target":  targetObj.Name,
			"action": "dns_configured",
		},
	}, format)

	// Cross-suggest: if Traefik is not configured for this app, suggest it
	suggestMissingTraefikSetup(*id)
}

// handleSetupTraefikCLI handles: hotify-cli setup-traefik --id <id> [--challenge-type http|dns] [--no-redirect] [--dry-run] [--target <name>] [--local]
func handleSetupTraefikCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-traefik", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	challengeType := cmd.String("challenge-type", "http", "ACME challenge type: http or dns (default: http)")
	noRedirect := cmd.Bool("no-redirect", false, "Disable HTTP-to-HTTPS redirect (useful for ACME troubleshooting)")
	dryRun := cmd.Bool("dry-run", false, "Preview changes without applying (diff)")
	rateLimit := cmd.String("rate-limit", "", "Rate limit: 'count,period' e.g. '10,60m' (stored in config, applied via setup-traefik)")
	target := cmd.String("target", "", "Target name (uses default if not specified)")
	local := cmd.Bool("local", false, "Execute directly on local server")
	cmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli setup-traefik --id <app-id>",
					"hotify-cli setup-traefik --id <app-id> --challenge-type dns",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	// Validate challenge type
	var ct TraefikChallengeType
	switch *challengeType {
	case "http", "":
		ct = ChallengeHTTP
	case "dns":
		ct = ChallengeDNS
	default:
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     fmt.Sprintf("Invalid --challenge-type '%s': must be 'http' or 'dns'", *challengeType),
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli setup-traefik --id <id> --challenge-type http",
					"hotify-cli setup-traefik --id <id> --challenge-type dns",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if *local {
		// Local mode: execute directly on local server

		// If --rate-limit was provided, save it to config (before dry-run, so both see it)
		if *rateLimit != "" {
			cfg, err := loadConfig()
			if err == nil {
				for i := range cfg.Apps {
					if cfg.Apps[i].ID == *id {
						cfg.Apps[i].RateLimit = *rateLimit
						saveConfig(cfg)
						break
					}
				}
			}
		}

		// If --dry-run, show diff and exit without making changes
		if *dryRun {
			config, err := loadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(ExitGenericFailure)
			}

			diff, err := DryRunDiff(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error computing diff: %v\n", err)
				os.Exit(ExitGenericFailure)
			}

			if diff == "" {
				fmt.Println("No changes — current config matches proposed config.")
			} else {
				fmt.Println("📋 Proposed Traefik changes (--dry-run):")
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

		var err error
		if *noRedirect {
			// Use explicit redirect control when flag is provided
			err = setupTraefikForAppWithChallengeAndRedirect(*id, ct, false, false)
		} else {
			// Use smart redirect handling by default
			err = setupTraefikForAppWithChallenge(*id, ct, false)
		}
		
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{
					Code: ExitTraefikConfigInvalid, Type: "traefik_error",
					Message:     err.Error(),
					Recoverable: true,
					Suggestions: []string{
						"Check Traefik is installed: hotify-cli traefik-system --status",
						"Verify app config: hotify-cli list",
						"Check Traefik logs: sudo journalctl -u traefik -f",
						"Try --challenge-type dns if HTTP challenge fails",
						"Try --no-redirect for ACME troubleshooting",
					},
				},
			}, format)
			os.Exit(ExitTraefikConfigInvalid)
		}
		printOutput(CommandResult{
			Version: Version,
			Success: true,
			Data: map[string]interface{}{
				"app_id":         *id,
				"challenge_type": string(ct),
				"redirect_enabled": !*noRedirect,
				"action":         "traefik_configured",
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

	if err := client.SetupTraefikApp(*id, string(ct), *noRedirect); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "remote_error",
				Message:     fmt.Sprintf("Failed to setup Traefik remotely: %v", err),
				Recoverable: true,
				Suggestions: []string{
					"Check target connectivity",
					"Verify hotify daemon is running on remote",
					"Check app exists on remote",
					"Try --challenge-type dns if HTTP challenge fails",
					"Try --no-redirect for ACME troubleshooting",
				},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":           *id,
			"target":            targetObj.Name,
			"challenge_type":    string(ct),
			"redirect_enabled":  !*noRedirect,
			"action":            "traefik_configured",
		},
	}, format)

	// Cross-suggest: if DNS is not configured, suggest it
	suggestMissingDNSSetup(*id)
}

// ─── Cross-suggestion helpers ───────────────────────────────────────────

// suggestMissingDNSSetup checks if the app's domain has a DNS record in
// Cloudflare. If not, it prints a suggestion to run setup-dns.
func suggestMissingDNSSetup(appID string) {
	config, err := loadConfig()
	if err != nil {
		return
	}

	var app *App
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			app = &config.Apps[i]
			break
		}
	}
	if app == nil || app.Domain == "" {
		return
	}

	// Check if DNS record exists via Cloudflare
	if config.CloudflareToken == "" || config.AdminEmail == "" {
		return
	}

	zoneID, err := getZoneID(app.Domain, config.CloudflareToken, config.AdminEmail)
	if err != nil {
		return
	}

	if _, _, found := existingDNSRecord(app.Domain, zoneID, config.CloudflareToken, config.AdminEmail); !found {
		fmt.Fprintf(os.Stderr, "\n⚠️  No DNS record found for %s.\n", app.Domain)
		fmt.Fprintf(os.Stderr, "   Run: hotify-cli setup-dns --id %s [--ip <public-ip>]\n\n", appID)
	}
}

// suggestMissingTraefikSetup checks if the app has a router in Traefik's
// dynamic config. If not, it prints a suggestion to run setup-traefik.
func suggestMissingTraefikSetup(appID string) {
	config, err := loadConfig()
	if err != nil {
		return
	}

	var app *App
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			app = &config.Apps[i]
			break
		}
	}
	if app == nil {
		return
	}

	// Check if Traefik dynamic config has a router for this app
	data, err := os.ReadFile(traefikDynamic)
	if err != nil {
		return
	}

	var dyn struct {
		HTTP struct {
			Routers map[string]interface{} `yaml:"routers"`
		} `yaml:"http"`
	}
	if err := yaml.Unmarshal(data, &dyn); err != nil {
		return
	}

	if _, exists := dyn.HTTP.Routers[appID]; !exists {
		fmt.Fprintf(os.Stderr, "\n⚠️  No Traefik route found for app '%s'.\n", appID)
		fmt.Fprintf(os.Stderr, "   Run: hotify-cli setup-traefik --id %s [--challenge-type dns]\n\n", appID)
	}
}
