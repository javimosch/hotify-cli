package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// handleCLIAppStart handles: hotify-cli start --id <id> [--target <name>]
func handleCLIAppStart() {
	format := getOutputFormat()
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	id := startCmd.String("id", "", "App ID to start")
	target := startCmd.String("target", "", "Target name (uses default if not specified)")
	startCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		// No --id: fall through to daemon handling in main.go
		handleDaemonStart()
		return
	}

	targetObj, err := getActiveTarget(*target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
				Recoverable: false,
				Suggestions: []string{"hotify-cli targets --action list", "hotify-cli targets --action use --name <name>"},
			},
		}, format)
		os.Exit(ExitTargetNotFound)
	}

	handleRemoteStart(*id, targetObj, format)
}

// handleCLIAppStop handles: hotify-cli stop --id <id> [--target <name>]
// Sends SIGTERM via the remote API; the remote server handles SIGKILL fallback.
func handleCLIAppStop() {
	format := getOutputFormat()
	stopCmd := flag.NewFlagSet("stop", flag.ExitOnError)
	id := stopCmd.String("id", "", "App ID to stop")
	target := stopCmd.String("target", "", "Target name (uses default if not specified)")
	stopCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		stopDaemon()
		return
	}

	targetObj, err := getActiveTarget(*target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
				Recoverable: false,
				Suggestions: []string{"hotify-cli targets --action list"},
			},
		}, format)
		os.Exit(ExitTargetNotFound)
	}

	handleRemoteStop(*id, targetObj, format)
}

// handleCLIAppRestart handles: hotify-cli restart --id <id> [--target <name>]
func handleCLIAppRestart() {
	format := getOutputFormat()
	restartCmd := flag.NewFlagSet("restart", flag.ExitOnError)
	id := restartCmd.String("id", "", "App ID to restart")
	target := restartCmd.String("target", "", "Target name")
	restartCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli restart --id <id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	targetObj, err := getActiveTarget(*target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitTargetNotFound)
	}

	handleRemoteRestart(*id, targetObj, format)
}

// handleCLIAppStatus handles: hotify-cli status --id <id> [--target <name>]
// Without --id, shows hotify daemon status.
func handleCLIAppStatus() {
	format := getOutputFormat()
	statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
	id := statusCmd.String("id", "", "App ID to check")
	target := statusCmd.String("target", "", "Target name")
	statusCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" {
		checkDaemonStatus()
		return
	}

	targetObj, err := getActiveTarget(*target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
				Recoverable: false,
			},
		}, format)
		os.Exit(ExitTargetNotFound)
	}

	handleRemoteStatus(*id, targetObj, format)
}

// handleDaemonStart starts the hotify UI daemon (called when start has no --id)
func handleDaemonStart() {
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	port := startCmd.Int("port", 8080, "Port for HTTP server")
	daemon := startCmd.Bool("daemon", false, "Run as daemon")
	startCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *daemon {
		startDaemon(*port)
	} else {
		startServer(*port)
	}
}

// outputAppAction is a shared helper for start/stop/restart success responses
func outputAppAction(appID, targetName, action, status string, format OutputFormat) {
	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id": appID,
			"target": targetName,
			"action": action,
			"status": status,
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}, format)
}

// handlePrune handles: hotify-cli prune [--id <id> | --all]
func handlePrune() {
	format := getOutputFormat()
	pruneCmd := flag.NewFlagSet("prune", flag.ExitOnError)
	id := pruneCmd.String("id", "", "App ID to prune (removes DNS + Traefik config for this app)")
	all := pruneCmd.Bool("all", false, "Prune all removed apps (clean up DNS + Traefik for apps no longer in config)")
	pruneCmd.Parse(filterHumanFlag(os.Args[2:]))

	if *id == "" && !*all {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Specify --id <app-id> or --all",
				Recoverable: false,
				Suggestions: []string{
					"hotify-cli prune --id <app-id>   # remove DNS+Traefik for one app",
					"hotify-cli prune --all           # clean up all orphaned DNS+Traefik configs",
				},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitConfigError)
	}

	results := []map[string]interface{}{}
	warnings := []string{}

	if *id != "" {
		r, w := pruneApp(*id, config)
		results = append(results, r)
		warnings = append(warnings, w...)
	} else {
		// --all: rebuild Traefik dynamic config without any individual DNS removal
		// (we don't track which DNS records were created, so we can only warn)
		warnings = append(warnings,
			"DNS records in Cloudflare are NOT automatically removed by --all",
			"Traefik dynamic.yml has been regenerated from current app list",
			"Manually remove stale DNS records from your Cloudflare dashboard",
		)
		if err := updateDynamicConfig(config); err != nil {
			warnings = append(warnings, fmt.Sprintf("Traefik config update failed: %v", err))
		} else {
			if err := restartTraefik(); err != nil {
				warnings = append(warnings, fmt.Sprintf("Traefik restart failed: %v", err))
			}
		}
		results = append(results, map[string]interface{}{"action": "rebuild_traefik", "status": "done"})
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data:    map[string]interface{}{"pruned": results},
		Metadata: map[string]interface{}{
			"warnings": warnings,
		},
	}, format)
}

// pruneApp removes Traefik routing for a single appID and warns about DNS.
func pruneApp(appID string, config *Config) (map[string]interface{}, []string) {
	warnings := []string{
		fmt.Sprintf("DNS A record for app '%s' was NOT removed from Cloudflare — remove it manually", appID),
	}

	// Rebuild Traefik dynamic config excluding this appID
	filtered := []App{}
	for _, app := range config.Apps {
		if app.ID != appID {
			filtered = append(filtered, app)
		}
	}
	tmpConfig := *config
	tmpConfig.Apps = filtered

	if err := updateDynamicConfig(&tmpConfig); err != nil {
		warnings = append(warnings, fmt.Sprintf("Traefik config update failed: %v", err))
		return map[string]interface{}{"app_id": appID, "status": "partial"}, warnings
	}

	if err := restartTraefik(); err != nil {
		warnings = append(warnings, fmt.Sprintf("Traefik restart failed: %v", err))
		return map[string]interface{}{"app_id": appID, "status": "partial"}, warnings
	}

	return map[string]interface{}{"app_id": appID, "status": "traefik_cleaned"}, warnings
}
