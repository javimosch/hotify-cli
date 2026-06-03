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
//   hotify-cli compose [--id <app>] [--target <t>] <subcommand> [args...]
//   hotify-cli compose <subcommand> [args...]   (no app context, raw passthrough)
//
// When --id is supplied, hotify-cli resolves compose_path and compose_file from
// the app config and changes into compose_path before running docker compose.
// When --target is supplied, the command is sent to the remote daemon.
// Any remaining args (including the subcommand) are forwarded verbatim.
func handleComposeCLI() {
	if len(os.Args) < 3 {
		printComposeHelp()
		os.Exit(1)
	}

	// Parse --id and --target from early args, leaving remaining as passthrough
	var appID string
	var targetName string
	passthroughArgs := os.Args[2:]
	filtered := []string{}

	for i := 0; i < len(passthroughArgs); i++ {
		arg := passthroughArgs[i]
		if (arg == "--id" || arg == "-id") && i+1 < len(passthroughArgs) {
			appID = passthroughArgs[i+1]
			i++
		} else if (arg == "--target") && i+1 < len(passthroughArgs) {
			targetName = passthroughArgs[i+1]
			i++
		} else if arg == "--local" {
			targetName = ""
		} else {
			filtered = append(filtered, arg)
		}
	}
	passthroughArgs = filtered

	if len(passthroughArgs) == 0 {
		printComposeHelp()
		os.Exit(1)
	}

	// help shortcut
	if passthroughArgs[0] == "help" || passthroughArgs[0] == "--help" || passthroughArgs[0] == "-h" {
		printComposeHelp()
		return
	}

	// Remote mode: forward to daemon
	if targetName != "" {
		handleComposeRemote(appID, targetName, passthroughArgs)
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

// handleComposeRemote sends a compose exec request to a remote daemon.
func handleComposeRemote(appID, targetName string, passthroughArgs []string) {
	format := getOutputFormat()
	target, err := getActiveTarget(targetName)
	if err != nil {
		exitTargetNotFound(format, err)
	}
	client, err := NewDeploymentClient(target)
	if err != nil {
		exitClientError(format, err)
	}

	if len(passthroughArgs) == 0 {
		printComposeHelp()
		os.Exit(1)
	}

	subcommand := passthroughArgs[0]
	args := []string{}
	if len(passthroughArgs) > 1 {
		args = passthroughArgs[1:]
	}

	result, err := client.ComposeExecRemote(appID, subcommand, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hotify-cli compose: remote error: %v\n", err)
		os.Exit(ExitGenericFailure)
	}

	// Print output directly (like a passthrough would)
	if output, ok := result["output"].(string); ok && output != "" {
		fmt.Print(output)
	}

	exitCode := 0
	if ec, ok := result["exit_code"].(float64); ok {
		exitCode = int(ec)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
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
