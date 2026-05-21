package main

import (
	"fmt"
	"os"
)

const Version = "2.7.2"

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

	// Traefik/DNS configuration
	case "setup-dns":
		handleSetupDNSCLI()
	case "setup-traefik":
		handleSetupTraefikCLI()

	// Docker container management
	case "docker":
		handleDockerCLI()

	// Docker Compose passthrough
	case "compose":
		handleComposeCLI()

	// Docker Compose deployment automation
	case "deploy-compose":
		handleDeployCompose()
	case "compose-sync":
		handleComposeSyncCLI()
	case "compose-copy-dir":
		handleComposeCopyDirCLI()
	case "volume-init":
		handleVolumeInitCLI()
	case "setup-compose":
		handleSetupComposeCLI()

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
	fmt.Println("DNS & Traefik:")
	fmt.Println("  setup-dns       Create/update Cloudflare DNS A record for an app")
	fmt.Println("  setup-traefik   Configure Traefik routing for an app")
	fmt.Println("                  --challenge-type http|dns  (default: http)")
	fmt.Println()
	fmt.Println("Docker:")
	fmt.Println("  docker list              List all containers")
	fmt.Println("  docker start <id>       Start a container")
	fmt.Println("  docker stop <id>        Stop a container")
	fmt.Println("  docker restart <id>     Restart a container")
	fmt.Println("  docker status <id>      Container status")
	fmt.Println("  docker logs <id>        Container logs")
	fmt.Println("  docker enable-traefik   Enable Traefik Docker provider")
	fmt.Println("  docker disable-traefik  Disable Traefik Docker provider")
	fmt.Println()
	fmt.Println("Docker Compose (passthrough):")
	fmt.Println("  compose [--id <app>] <subcommand> [args...]")
	fmt.Println("  When --id is set, resolves compose_path and compose_file from app config")
	fmt.Println("  Examples:")
	fmt.Println("    hotify-cli compose --id cmdcenter up -d")
	fmt.Println("    hotify-cli compose --id cmdcenter logs -f")
	fmt.Println("    hotify-cli compose --id cmdcenter down")
	fmt.Println()
	fmt.Println("Docker Compose Deployment Automation:")
	fmt.Println("  deploy-compose      Copy full project tree to remote compose_path")
	fmt.Println("                      --id <app> --source <local-dir> [--compose-file <file>]")
	fmt.Println("                      [--remote-path <path>] [--start]")
	fmt.Println("  compose-sync        Sync compose file (+ .env) to remote compose_path")
	fmt.Println("                      --id <app> [--source <dir>] [--restart] [--env=false]")
	fmt.Println("  compose-copy-dir    Copy a local directory into remote compose_path")
	fmt.Println("                      --id <app> --dir <subdir-name> --source <local-dir>")
	fmt.Println("  volume-init         Populate a Docker named volume with local directory")
	fmt.Println("                      --id <app> --volume <vol-name> --source <local-dir>")
	fmt.Println("                      NOTE: remote daemon needs write access to /var/lib/docker/volumes/")
	fmt.Println("  setup-compose       Register app + deploy project files in one command")
	fmt.Println("                      --id <app> --name <n> --domain <d> --port <p> --cmd <c>")
	fmt.Println("                      --source <dir> [--compose-file <f>] [--remote-path <p>]")
	fmt.Println("                      [--setup-dns] [--start]")
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
	fmt.Println()
	fmt.Println("  # Setup Docker Compose app")
	fmt.Println("  hotify-cli setup --id cmdcenter --name 'CmdCenter' --domain cmdcenter --port 3031 --cmd 'docker compose up -d' --compose-file compose.binary.yml --compose-path /home/dk1/cmdcenter")
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
