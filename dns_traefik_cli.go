package main

import (
	"flag"
	"fmt"
	"os"
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
}

// handleSetupTraefikCLI handles: hotify-cli setup-traefik --id <id> [--challenge-type http|dns] [--no-redirect] [--target <name>] [--local]
func handleSetupTraefikCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-traefik", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	challengeType := cmd.String("challenge-type", "http", "ACME challenge type: http or dns (default: http)")
	noRedirect := cmd.Bool("no-redirect", false, "Disable HTTP-to-HTTPS redirect (useful for ACME troubleshooting)")
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
}
