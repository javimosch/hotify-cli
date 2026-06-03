package main

import (
	"flag"
	"fmt"
	"os"
	"time"
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
	fmt.Println("  hotify-cli docker list [--target <t>] [--local]       List all containers")
	fmt.Println("  hotify-cli docker start --id <id> [--target <t>]      Start a container")
	fmt.Println("  hotify-cli docker stop --id <id> [--target <t>]       Stop a container")
	fmt.Println("  hotify-cli docker restart --id <id> [--target <t>]    Restart a container")
	fmt.Println("  hotify-cli docker status --id <id> [--target <t>]     Container status")
	fmt.Println("  hotify-cli docker logs --id <id> [--target <t>]       Container logs (last 50 lines)")
	fmt.Println("  hotify-cli docker enable-traefik [--target <t>]       Enable Traefik Docker provider")
	fmt.Println("  hotify-cli docker disable-traefik [--target <t>]      Disable Traefik Docker provider")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --target <name>   Execute on remote target")
	fmt.Println("  --local           Execute locally (default when no target configured)")
}

// dockerTargetOrLocal resolves target and local flags from os.Args[3:].
func dockerTargetOrLocal() (target *Remote, isLocal bool, format OutputFormat) {
	fs := flag.NewFlagSet("docker-flags", flag.ContinueOnError)
	targetName := fs.String("target", "", "Target name")
	localFlag := fs.Bool("local", false, "Execute locally")
	// ignore errors: subcommand-specific flags parsed separately
	_ = fs.Parse(filterHumanFlag(os.Args[3:]))
	format = getOutputFormat()
	if *localFlag {
		return nil, true, format
	}
	if *targetName != "" {
		t, err := getActiveTarget(*targetName)
		if err != nil {
			printOutput(CommandResult{
				Version: Version, Success: false,
				Error: &CommandError{Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(), Recoverable: false},
			}, format)
			os.Exit(ExitTargetNotFound)
		}
		return t, false, format
	}
	return nil, true, format // default local
}

func handleDockerList() {
	target, isLocal, format := dockerTargetOrLocal()

	if !isLocal && target != nil {
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		result, err := client.DockerListRemote()
		if err != nil {
			printOutput(CommandResult{Version: Version, Success: false,
				Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: result, Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()}}, format)
		return
	}

	containers, err := dockerList()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true,
				Suggestions: []string{"Check Docker is installed", "Check Docker daemon is running"}},
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
	printOutput(CommandResult{Version: Version, Success: true,
		Data: map[string]interface{}{"containers": containers, "count": len(containers)}}, format)
}

func handleDockerStart() {
	cmd := flag.NewFlagSet("docker start", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	targetName := cmd.String("target", "", "Target name")
	local := cmd.Bool("local", false, "Execute locally")
	cmd.Parse(filterHumanFlag(os.Args[3:]))
	format := getOutputFormat()

	if *id == "" {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "validation_error", Message: "Missing required flag: --id",
				Recoverable: false, Suggestions: []string{"hotify-cli docker start --id <container-id>"}}}, format)
		os.Exit(ExitInvalidArgument)
	}

	if !*local && *targetName != "" {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if err := client.DockerStartRemote(*id); err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: map[string]interface{}{"container_id": *id, "action": "started", "target": target.Name},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()}}, format)
		return
	}

	if err := dockerStart(*id); err != nil {
		printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"container_id": *id, "action": "started"}}, format)
}

func handleDockerStop() {
	cmd := flag.NewFlagSet("docker stop", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	targetName := cmd.String("target", "", "Target name")
	local := cmd.Bool("local", false, "Execute locally")
	cmd.Parse(filterHumanFlag(os.Args[3:]))
	format := getOutputFormat()

	if *id == "" {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "validation_error", Message: "Missing required flag: --id",
				Recoverable: false, Suggestions: []string{"hotify-cli docker stop --id <container-id>"}}}, format)
		os.Exit(ExitInvalidArgument)
	}

	if !*local && *targetName != "" {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if err := client.DockerStopRemote(*id); err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: map[string]interface{}{"container_id": *id, "action": "stopped", "target": target.Name},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()}}, format)
		return
	}

	if err := dockerStop(*id); err != nil {
		printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"container_id": *id, "action": "stopped"}}, format)
}

func handleDockerRestart() {
	cmd := flag.NewFlagSet("docker restart", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	targetName := cmd.String("target", "", "Target name")
	local := cmd.Bool("local", false, "Execute locally")
	cmd.Parse(filterHumanFlag(os.Args[3:]))
	format := getOutputFormat()

	if *id == "" {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "validation_error", Message: "Missing required flag: --id",
				Recoverable: false, Suggestions: []string{"hotify-cli docker restart --id <container-id>"}}}, format)
		os.Exit(ExitInvalidArgument)
	}

	if !*local && *targetName != "" {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if err := client.DockerRestartRemote(*id); err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: map[string]interface{}{"container_id": *id, "action": "restarted", "target": target.Name},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()}}, format)
		return
	}

	if err := dockerRestart(*id); err != nil {
		printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"container_id": *id, "action": "restarted"}}, format)
}

func handleDockerStatus() {
	cmd := flag.NewFlagSet("docker status", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	targetName := cmd.String("target", "", "Target name")
	local := cmd.Bool("local", false, "Execute locally")
	cmd.Parse(filterHumanFlag(os.Args[3:]))
	format := getOutputFormat()

	if *id == "" {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "validation_error", Message: "Missing required flag: --id",
				Recoverable: false, Suggestions: []string{"hotify-cli docker status --id <container-id>"}}}, format)
		os.Exit(ExitInvalidArgument)
	}

	if !*local && *targetName != "" {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		result, err := client.DockerStatusRemote(*id)
		if err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true, Data: result, Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()}}, format)
		return
	}

	container, err := dockerStatus(*id)
	if err != nil {
		printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitGenericFailure)
	}
	printOutput(CommandResult{Version: Version, Success: true,
		Data: map[string]interface{}{"id": container.ID, "name": container.Name, "image": container.Image,
			"status": container.Status, "ports": container.Ports, "labels": container.Labels}}, format)
}

func handleDockerLogs() {
	cmd := flag.NewFlagSet("docker logs", flag.ExitOnError)
	id := cmd.String("id", "", "Container ID or name (required)")
	tail := cmd.Int("tail", 50, "Number of log lines (default: 50)")
	targetName := cmd.String("target", "", "Target name")
	local := cmd.Bool("local", false, "Execute locally")
	cmd.Parse(filterHumanFlag(os.Args[3:]))
	format := getOutputFormat()

	if *id == "" {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "validation_error", Message: "Missing required flag: --id",
				Recoverable: false, Suggestions: []string{"hotify-cli docker logs --id <container-id>"}}}, format)
		os.Exit(ExitInvalidArgument)
	}

	if !*local && *targetName != "" {
		target, err := getActiveTarget(*targetName)
		if err != nil {
			exitTargetNotFound(format, err)
		}
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		result, err := client.DockerLogsRemote(*id, *tail)
		if err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true, Data: result, Metadata: map[string]interface{}{"target": target.Name, "timestamp": time.Now().Unix()}}, format)
		return
	}

	logs, err := dockerLogs(*id, *tail)
	if err != nil {
		printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "docker_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitGenericFailure)
	}
	if format == OutputFormatText {
		fmt.Println(logs)
		return
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"container_id": *id, "logs": logs}}, format)
}

func handleDockerEnableTraefik() {
	target, isLocal, format := dockerTargetOrLocal()

	if !isLocal && target != nil {
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if err := client.DockerEnableTraefikRemote(); err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: map[string]interface{}{"action": "docker_provider_enabled", "target": target.Name},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()}}, format)
		return
	}

	if err := enableDockerProvider(); err != nil {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitTraefikConfigInvalid, Type: "traefik_error", Message: err.Error(), Recoverable: true,
				Suggestions: []string{"Check Traefik is installed", "Check Docker is running"}}}, format)
		os.Exit(ExitTraefikConfigInvalid)
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"action": "docker_provider_enabled"}}, format)
}

func handleDockerDisableTraefik() {
	target, isLocal, format := dockerTargetOrLocal()

	if !isLocal && target != nil {
		client, err := NewDeploymentClient(target)
		if err != nil {
			exitClientError(format, err)
		}
		if err := client.DockerDisableTraefikRemote(); err != nil {
			printOutput(CommandResult{Version: Version, Success: false, Error: &CommandError{Code: ExitGenericFailure, Type: "remote_error", Message: err.Error(), Recoverable: true}}, format)
			os.Exit(ExitGenericFailure)
		}
		printOutput(CommandResult{Version: Version, Success: true,
			Data: map[string]interface{}{"action": "docker_provider_disabled", "target": target.Name},
			Metadata: map[string]interface{}{"timestamp": time.Now().Unix()}}, format)
		return
	}

	if err := disableDockerProvider(); err != nil {
		printOutput(CommandResult{Version: Version, Success: false,
			Error: &CommandError{Code: ExitTraefikConfigInvalid, Type: "traefik_error", Message: err.Error(), Recoverable: true}}, format)
		os.Exit(ExitTraefikConfigInvalid)
	}
	printOutput(CommandResult{Version: Version, Success: true, Data: map[string]interface{}{"action": "docker_provider_disabled"}}, format)
}
