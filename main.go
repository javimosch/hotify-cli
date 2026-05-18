package main

import (
	"fmt"
	"os"
)

const Version = "2.1.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	// Config & setup
	case "init":
		initConfig()
	case "setup":
		setupApp(true) // upsert: create or update
	case "add":
		addApp() // strict create (fails if ID exists)
	case "edit":
		editApp() // backward compat alias → setup upsert
	case "remove":
		removeApp()
	case "list":
		listApps()

	// App process management (--id required; without --id targets the hotify daemon)
	case "start":
		handleCLIAppStart()
	case "stop":
		handleCLIAppStop()
	case "restart":
		handleCLIAppRestart()
	case "status":
		handleCLIAppStatus()
	case "pause":
		handleCLIAppPause()
	case "resume":
		handleCLIAppResume()

	// File deployment
	case "deploy":
		handleDeploy()

	// Infrastructure
	case "prune":
		handlePrune()
	case "traefik-system":
		handleTraefikSystem()

	// Remote target & auth management
	case "auth":
		handleAuth()
	case "targets":
		handleTargets()
	case "api-keys":
		handleAPIKeysCLI()

	// Misc
	case "version":
		fmt.Printf("hotify-cli v%s\n", Version)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("hotify-cli v" + Version + " — Traefik/Cloudflare app management CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hotify-cli <command> [options]")
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Println("  init            Initialize config (non-interactive in JSON mode)")
	fmt.Println("  setup           Create or update an app (upsert)")
	fmt.Println("  add             Add a new app (fails if ID already exists)")
	fmt.Println("  edit            Update an existing app (alias for setup)")
	fmt.Println("  remove          Remove app from config (warns about DNS/Traefik cleanup)")
	fmt.Println("  list            List all configured apps")
	fmt.Println()
	fmt.Println("Process Management (--id targets a remote app; no --id targets hotify daemon):")
	fmt.Println("  start           Start app or hotify daemon (--daemon for daemon mode)")
	fmt.Println("  stop            Stop app or hotify daemon")
	fmt.Println("  restart         Restart app (requires --id)")
	fmt.Println("  status          App or daemon status")
	fmt.Println("  pause           Pause app (SIGSTOP, requires --id)")
	fmt.Println("  resume          Resume paused app (SIGCONT, requires --id)")
	fmt.Println()
	fmt.Println("Deployment:")
	fmt.Println("  deploy          Upload binary/folder to remote target")
	fmt.Println()
	fmt.Println("Cleanup:")
	fmt.Println("  prune           Remove DNS/Traefik config for an app or globally")
	fmt.Println()
	fmt.Println("Infrastructure:")
	fmt.Println("  traefik-system  Manage Traefik installation on targets")
	fmt.Println("  auth            Authenticate with remote hotify daemon")
	fmt.Println("  targets         Manage deployment targets")
	fmt.Println("  api-keys        Manage API keys on remote daemon")
	fmt.Println()
	fmt.Println("Output:")
	fmt.Println("  Default output is JSON (agent-friendly)")
	fmt.Println("  Add --human for human-readable text output")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Initialize (non-interactive)")
	fmt.Println("  hotify-cli init --token <cf-token> --domain example.com --email admin@example.com")
	fmt.Println()
	fmt.Println("  # Setup app (create or update)")
	fmt.Println("  hotify-cli setup --id myapp --name 'My App' --domain myapp --port 3000 --cmd '/usr/local/bin/myapp start'")
	fmt.Println("  hotify-cli setup --id myapp --port 4000   # update port only")
	fmt.Println("  hotify-cli setup --id myapp --domain myapp --port 3000 --setup-dns  # with auto DNS")
	fmt.Println("  hotify-cli setup --id myapp --domain myapp --port 3000 --setup-dns --ip 1.2.3.4")
	fmt.Println()
	fmt.Println("  # Deploy binary and optionally set up DNS")
	fmt.Println("  hotify-cli deploy --id myapp --source ./myapp-binary")
	fmt.Println("  hotify-cli deploy --id myapp --source ./myapp-binary --setup-dns")
	fmt.Println()
	fmt.Println("  # Process management")
	fmt.Println("  hotify-cli start   --id myapp")
	fmt.Println("  hotify-cli stop    --id myapp")
	fmt.Println("  hotify-cli restart --id myapp")
	fmt.Println("  hotify-cli status  --id myapp")
	fmt.Println("  hotify-cli pause   --id myapp")
	fmt.Println("  hotify-cli resume  --id myapp")
	fmt.Println()
	fmt.Println("  # Hotify daemon")
	fmt.Println("  hotify-cli start --daemon")
	fmt.Println("  hotify-cli stop")
	fmt.Println("  hotify-cli status")
	fmt.Println()
	fmt.Println("  # Cleanup after remove")
	fmt.Println("  hotify-cli remove --id myapp")
	fmt.Println("  hotify-cli prune  --id myapp   # removes Traefik config, warns about DNS")
	fmt.Println("  hotify-cli prune  --all         # rebuilds Traefik for current app list")
	fmt.Println()
	fmt.Println("  # Targets & auth")
	fmt.Println("  hotify-cli auth --url http://server:3060 --token xxx --name myserver")
	fmt.Println("  hotify-cli targets --action use --name myserver")
}
