package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// handleLocalStart starts an app locally by executing its command and tracking PID
func handleLocalStart(appID string, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
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
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "not_found", Message: "App not found", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	// Execute the command
	cmd := exec.Command("sh", "-c", app.Command)
	if err := cmd.Start(); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "exec_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	// Update config with PID and status
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			config.Apps[i].PID = cmd.Process.Pid
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": "running", "pid": cmd.Process.Pid},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

// handleLocalStop stops an app locally using SIGTERM
func handleLocalStop(appID string, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
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
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "not_found", Message: "App not found", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if app.PID == 0 {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "invalid_state", Message: "PID not tracked for this app", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	process, err := os.FindProcess(app.PID)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "process_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "signal_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	// Update config
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			config.Apps[i].Status = "stopped"
			config.Apps[i].PID = 0
			saveConfig(config)
			break
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": "stopped", "pid": app.PID, "signal": "SIGTERM"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

// handleLocalRestart restarts an app locally (stop + start)
func handleLocalRestart(appID string, format OutputFormat) {
	handleLocalStop(appID, format)
	time.Sleep(1 * time.Second)
	handleLocalStart(appID, format)
}

// handleLocalStatus returns app status from local config
func handleLocalStatus(appID string, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
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
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "not_found", Message: "App not found", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": app.Status, "pid": app.PID},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

// handleLocalPause pauses an app locally using SIGSTOP
func handleLocalPause(appID string, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
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
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "not_found", Message: "App not found", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if app.PID == 0 {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "invalid_state", Message: "PID not tracked for this app", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	process, err := os.FindProcess(app.PID)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "process_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := process.Signal(syscall.SIGSTOP); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "signal_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	// Update config
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			config.Apps[i].Status = "paused"
			saveConfig(config)
			break
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": "paused", "pid": app.PID, "signal": "SIGSTOP"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}

// handleLocalResume resumes a paused app locally using SIGCONT
func handleLocalResume(appID string, format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
		}, format)
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
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "not_found", Message: "App not found", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	if app.PID == 0 {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitInvalidArgument, Type: "invalid_state", Message: "PID not tracked for this app", Recoverable: false},
		}, format)
		os.Exit(ExitInvalidArgument)
	}

	process, err := os.FindProcess(app.PID)
	if err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "process_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	if err := process.Signal(syscall.SIGCONT); err != nil {
		printOutput(CommandResult{
			Version: Version, Success: false,
			Error: &CommandError{Code: ExitGenericFailure, Type: "signal_error", Message: err.Error(), Recoverable: false},
		}, format)
		os.Exit(ExitGenericFailure)
	}

	// Update config
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": "running", "pid": app.PID, "signal": "SIGCONT"},
		Metadata: map[string]interface{}{"timestamp": time.Now().Unix()},
	}, format)
}