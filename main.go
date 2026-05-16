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
	fmt.Println("  start       Start HTTP server (UI)")
	fmt.Println("  stop        Stop daemon server")
	fmt.Println("  status      Check daemon status")
	fmt.Println("  version     Show version information")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("Start Options:")
	fmt.Println("  -port int   Port for HTTP server (default 8080)")
	fmt.Println("  -daemon     Run as daemon (background)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  hotify-cli init")
	fmt.Println("  hotify-cli add --id myapp --name \"My App\" --domain myapp.example.com --port 3000 --command \"/path/to/app start\"")
	fmt.Println("  hotify-cli list")
	fmt.Println("  hotify-cli start -daemon")
	fmt.Println("  hotify-cli status")
}
