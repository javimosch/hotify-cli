package main

import (
	"fmt"
	"os"
	"time"
)

func handleRemoteStart(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Starting app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.StartApp(appID); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "start_error",
				Message:     fmt.Sprintf("Failed to start app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check app configuration", "Verify app is installed", "Check app logs"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{"app_id": appID, "target": target.Name, "action": "start", "status": "started"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

func handleRemoteStop(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Stopping app %s on target %s (SIGTERM)\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.StopApp(appID); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "stop_error",
				Message:     fmt.Sprintf("Failed to stop app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check if app is running", "Check app logs"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{"app_id": appID, "target": target.Name, "action": "stop", "status": "stopped", "signal": "SIGTERM"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

func handleRemoteRestart(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Restarting app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.RestartApp(appID); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "restart_error",
				Message:     fmt.Sprintf("Failed to restart app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check app configuration", "Verify app is installed", "Check app logs"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{"app_id": appID, "target": target.Name, "action": "restart", "status": "restarted"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

func handleRemoteStatus(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Checking status of app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	status, err := client.GetAppStatus(appID)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{
				Code: ExitGenericFailure, Type: "status_error",
				Message:     fmt.Sprintf("Failed to get status: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check if app exists", "Check target connectivity"},
			},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data: map[string]interface{}{"app_id": appID, "target": target.Name, "status": status},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}
