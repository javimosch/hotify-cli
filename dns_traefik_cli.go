package main

import (
	"flag"
	"fmt"
	"os"
)

// handleSetupDNSCLI handles: hotify-cli setup-dns --id <id> [--ip <ip>]
func handleSetupDNSCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-dns", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	ip := cmd.String("ip", "", "Server IP (auto-detected if omitted)")
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
}

// handleSetupTraefikCLI handles: hotify-cli setup-traefik --id <id> [--challenge-type http|dns]
func handleSetupTraefikCLI() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("setup-traefik", flag.ExitOnError)
	id := cmd.String("id", "", "App ID (required)")
	challengeType := cmd.String("challenge-type", "http", "ACME challenge type: http or dns (default: http)")
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

	if err := setupTraefikForAppWithChallenge(*id, ct, false); err != nil {
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
			"action":         "traefik_configured",
		},
	}, format)
}
