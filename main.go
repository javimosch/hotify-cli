package main

import (
	"flag"
	"fmt"
	"os"
)

const Version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "init":
		handleInit()
	case "add":
		handleAdd()
	case "edit":
		handleEdit()
	case "remove":
		handleRemove()
	case "list":
		handleList()
	case "auth":
		handleAuth()
	case "api-keys":
		handleAPIKeysCLI()
	case "targets":
		handleTargets()
	case "deploy":
		handleDeploy()
	case "start":
		handleStart()
	case "stop":
		handleStop()
	case "status":
		handleStatus()
	case "version":
		handleVersion()
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printHelp()
		os.Exit(1)
	}
}

func handleInit() {
	initConfig()
}

func handleAdd() {
	addApp()
}

func handleEdit() {
	editApp()
}

func handleRemove() {
	removeApp()
}

func handleList() {
	listApps()
}

func handleStart() {
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	port := startCmd.Int("port", 8080, "Port for HTTP server")
	daemon := startCmd.Bool("daemon", false, "Run as daemon")
	startCmd.Parse(os.Args[2:])

	if *daemon {
		startDaemon(*port)
	} else {
		startServer(*port)
	}
}

func handleStop() {
	stopDaemon()
}

func handleStatus() {
	checkDaemonStatus()
}

func handleVersion() {
	fmt.Printf("hotify-cli v%s\n", Version)
}

func printHelp() {
	fmt.Println("hotify-cli - Traefik/Cloudflare app management CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  hotify-cli <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init        Initialize configuration (CF token, domain, email)")
	fmt.Println("  add         Add a new app")
	fmt.Println("  edit        Edit an existing app")
	fmt.Println("  remove      Remove an app")
	fmt.Println("  list        List all apps")
	fmt.Println("  auth        Authenticate with remote daemon")
	fmt.Println("  targets     Manage deployment targets")
	fmt.Println("  deploy      Deploy and manage remote applications")
	fmt.Println("  api-keys    Manage API keys (remote daemon)")
	fmt.Println("  start       Start HTTP server (UI)")
	fmt.Println("  stop        Stop daemon server")
	fmt.Println("  status      Check daemon status")
	fmt.Println("  version     Show version information")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("Authentication Commands:")
	fmt.Println("  hotify-cli auth --url <url> --token <token> --name <name>")
	fmt.Println("  hotify-cli auth list")
	fmt.Println("  hotify-cli auth remove --name <name>")
	fmt.Println("  hotify-cli auth test --name <name>")
	fmt.Println()
	fmt.Println("Target Commands:")
	fmt.Println("  hotify-cli targets --action list")
	fmt.Println("  hotify-cli targets --action use --name <name>")
	fmt.Println("  hotify-cli targets --action validate [--name <name>]")
	fmt.Println("  hotify-cli targets --action remove --name <name>")
	fmt.Println()
	fmt.Println("Deploy Commands:")
	fmt.Println("  hotify-cli deploy --id <id> --source <path> [--target <name>]")
	fmt.Println("  hotify-cli deploy --id <id> --action start [--target <name>]")
	fmt.Println("  hotify-cli deploy --id <id> --action stop [--target <name>]")
	fmt.Println("  hotify-cli deploy --id <id> --action restart [--target <name>]")
	fmt.Println("  hotify-cli deploy --id <id> --action status [--target <name>]")
	fmt.Println()
	fmt.Println("API Key Commands:")
	fmt.Println("  hotify-cli api-keys add --name <name> [--token <token>] [--permissions <perms>]")
	fmt.Println("  hotify-cli api-keys list")
	fmt.Println("  hotify-cli api-keys remove --name <name>")
	fmt.Println("  hotify-cli api-keys regenerate --name <name>")
	fmt.Println("  hotify-cli api-keys permissions --name <name> [--add <perms>] [--remove <perms>]")
	fmt.Println()
	fmt.Println("Start Options:")
	fmt.Println("  -port int   Port for HTTP server (default 8080)")
	fmt.Println("  -daemon     Run as daemon (background)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hotify-cli init")
	fmt.Println("  hotify-cli add --id myapp --name \"My App\" --domain myapp.example.com --port 3000 --command \"/path/to/app start\"")
	fmt.Println("  hotify-cli auth --url http://dk1:3060 --token xxx --name dk1")
	fmt.Println("  hotify-cli targets --action use --name dk1")
	fmt.Println("  hotify-cli deploy --id myapp --source ./myapp  # Uses default target")
	fmt.Println("  hotify-cli deploy --id myapp --action start")
	fmt.Println("  hotify-cli auth test  # Uses default target")
	fmt.Println("  hotify-cli api-keys add --name \"local-machine\" --permissions \"deploy,start,stop\"")
	fmt.Println("  hotify-cli list")
	fmt.Println("  hotify-cli start -daemon")
	fmt.Println("  hotify-cli status")
}
