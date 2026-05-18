package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// handleComposeCLI handles the "compose" command group.
// It is a passthrough to "docker compose" with optional app-context resolution.
//
// Usage patterns:
//   hotify-cli compose --id <app> <subcommand> [args...]
//   hotify-cli compose <subcommand> [args...]   (no app context, raw passthrough)
//
// When --id is supplied, hotify-cli resolves compose_path and compose_file from
// the app config and changes into compose_path before running docker compose.
// Any remaining args (including the subcommand) are forwarded verbatim.
func handleComposeCLI() {
	if len(os.Args) < 3 {
		printComposeHelp()
		os.Exit(1)
	}

	// Check if first arg is --id or -id
	var appID string
	passthroughArgs := os.Args[2:]

	if len(passthroughArgs) >= 2 && (passthroughArgs[0] == "--id" || passthroughArgs[0] == "-id") {
		appID = passthroughArgs[1]
		passthroughArgs = passthroughArgs[2:]
	}

	if len(passthroughArgs) == 0 {
		printComposeHelp()
		os.Exit(1)
	}

	// help shortcut
	if passthroughArgs[0] == "help" || passthroughArgs[0] == "--help" || passthroughArgs[0] == "-h" {
		printComposeHelp()
		return
	}

	var workDir string
	var extraArgs []string // prepend -f <file> if compose_file is configured

	if appID != "" {
		config, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "hotify-cli compose: error loading config: %v\n", err)
			os.Exit(ExitConfigError)
		}

		var app *App
		for i := range config.Apps {
			if config.Apps[i].ID == appID {
				app = &config.Apps[i]
				break
			}
		}
		if app == nil {
			fmt.Fprintf(os.Stderr, "hotify-cli compose: app '%s' not found\n", appID)
			os.Exit(ExitInvalidArgument)
		}

		if app.ComposePath != "" {
			workDir = app.ComposePath
		}
		if app.ComposeFile != "" {
			extraArgs = append(extraArgs, "-f", app.ComposeFile)
		}
	}

	// If compose_path is set, resolve it (expand ~)
	if workDir != "" {
		if len(workDir) > 0 && workDir[0] == '~' {
			home, _ := os.UserHomeDir()
			workDir = filepath.Join(home, workDir[1:])
		}
	}

	// Build final docker compose args: [extraArgs...] [passthroughArgs...]
	dockerArgs := append([]string{"compose"}, extraArgs...)
	dockerArgs = append(dockerArgs, passthroughArgs...)

	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if workDir != "" {
		cmd.Dir = workDir
	}

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "hotify-cli compose: %v\n", err)
		os.Exit(ExitGenericFailure)
	}
}

func printComposeHelp() {
	fmt.Println("hotify-cli compose — passthrough to docker compose")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hotify-cli compose [--id <app>] <subcommand> [docker-compose-args...]")
	fmt.Println()
	fmt.Println("When --id is supplied, hotify-cli resolves compose_path and compose_file")
	fmt.Println("from the app config, then runs docker compose from that directory.")
	fmt.Println("The -f <compose_file> flag is prepended automatically if compose_file is set.")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # App-aware (resolves compose_path and compose_file automatically)")
	fmt.Println("  hotify-cli compose --id cmdcenter up -d")
	fmt.Println("  hotify-cli compose --id cmdcenter down")
	fmt.Println("  hotify-cli compose --id cmdcenter ps")
	fmt.Println("  hotify-cli compose --id cmdcenter logs -f")
	fmt.Println("  hotify-cli compose --id cmdcenter restart")
	fmt.Println("  hotify-cli compose --id cmdcenter pull")
	fmt.Println()
	fmt.Println("  # Raw passthrough (runs docker compose in current directory)")
	fmt.Println("  hotify-cli compose up -d")
	fmt.Println("  hotify-cli compose -f compose.binary.yml up -d")
	fmt.Println("  hotify-cli compose ps")
	fmt.Println("  hotify-cli compose logs")
	fmt.Println()
	fmt.Println("All subcommands and flags are forwarded verbatim to docker compose.")
}
