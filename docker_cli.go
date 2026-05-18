package main

import (
	"flag"
	"fmt"
	"os"
)

// handleDockerCLI handles the docker subcommands
func handleDockerCLI() {
	if len(os.Args) < 3 {
		printDockerHelp()
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "list":
		handleDockerList()
	case "start":
		handleDockerStart()
	case "stop":
		handleDockerStop()
	case "restart":
		handleDockerRestart()
	case "status":
		handleDockerStatus()
	case "logs":
		handleDockerLogs()
	case "enable-traefik":
		handleDockerEnableTraefik()
	case "disable-traefik":
		handleDockerDisableTraefik()
	case "help", "--help", "-h":
		printDockerHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown docker subcommand: %s\n", subcommand)
		printDockerHelp()
		os.Exit(1)
	}
}

func printDockerHelp() {
	fmt.Println("Docker commands:")
	fmt.Println("  hotify-cli docker list              List all containers")
	fmt.Println("  hotify-cli docker start <id>       Start a container")
	fmt.Println("  hotify-cli docker stop <id>        Stop a container")
	fmt.Println("  hotify-cli docker restart <id>     Restart a container")
	fmt.Println("  hotify-cli docker status <id>      Container status")
	fmt.Println("  hotify-cli docker logs <id>        Container logs (last 50 lines)")
	fmt.Println("  hotify-cli docker enable-traefik   Enable Traefik Docker provider")
	fmt.Println("  hotify-cli docker disable-traefik  Disable Traefik Docker provider")
}

func handleDockerList() {
	format := getOutputFormat()

	containers, err := dockerList()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
				Suggestions: []string{"Check Docker is installed", "Check Docker daemon is running"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if format == OutputFormatText {
		if len(containers) == 0 {
			fmt.Println("No containers found")
			return
		}
		fmt.Println("Containers:")
		for _, c := range containers {
			fmt.Printf("ID: %s\n", c.ID)
			fmt.Printf("  Name:   %s\n", c.Name)
			fmt.Printf("  Image:  %s\n", c.Image)
			fmt.Printf("  Status: %s\n", c.Status)
			if len(c.Ports) > 0 {
				fmt.Printf("  Ports:  %s\n", fmt.Sprintf("%v", c.Ports))
			}
			fmt.Println()
		}
		return
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"containers": containers,
			"count":      len(containers),
		},
	}, format)
}

func handleDockerStart() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("docker start", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	cmd.Parse(filterHumanFlag(os.Args[3:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli docker start --id <container-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if err := dockerStart(*id); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"container_id": *id,
			"action":       "started",
		},
	}, format)
}

func handleDockerStop() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("docker stop", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	cmd.Parse(filterHumanFlag(os.Args[3:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli docker stop --id <container-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if err := dockerStop(*id); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"container_id": *id,
			"action":       "stopped",
		},
	}, format)
}

func handleDockerRestart() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("docker restart", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	cmd.Parse(filterHumanFlag(os.Args[3:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli docker restart --id <container-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if err := dockerRestart(*id); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"container_id": *id,
			"action":       "restarted",
		},
	}, format)
}

func handleDockerStatus() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("docker status", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	cmd.Parse(filterHumanFlag(os.Args[3:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli docker status --id <container-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	container, err := dockerStatus(*id)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"id":     container.ID,
			"name":   container.Name,
			"image":  container.Image,
			"status": container.Status,
			"ports":  container.Ports,
			"labels": container.Labels,
		},
	}, format)
}

func handleDockerLogs() {
	format := getOutputFormat()
	cmd := flag.NewFlagSet("docker logs", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	tail := cmd.Int("tail", 50, "Number of log lines (default: 50)")
	cmd.Parse(filterHumanFlag(os.Args[3:]))

	if *id == "" {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitInvalidArgument, Type: "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"hotify-cli docker logs --id <container-id>"},
			},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	logs, err := dockerLogs(*id, *tail)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "docker_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if format == OutputFormatText {
		fmt.Println(logs)
		return
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"container_id": *id,
			"logs":         logs,
		},
	}, format)
}

func handleDockerEnableTraefik() {
	format := getOutputFormat()

	if err := enableDockerProvider(); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTraefikConfigInvalid, Type: "traefik_error",
				Message:     err.Error(),
				Recoverable: true,
				Suggestions: []string{"Check Traefik is installed", "Check Docker is running"},
			},
		}, format)
		os.Exit(ExitTraefikConfigInvalid)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"action": "docker_provider_enabled",
		},
	}, format)
}

func handleDockerDisableTraefik() {
	format := getOutputFormat()

	if err := disableDockerProvider(); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitTraefikConfigInvalid, Type: "traefik_error",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, format)
		os.Exit(ExitTraefikConfigInvalid)
	}

	printOutput(CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"action": "docker_provider_disabled",
		},
	}, format)
}
