package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	pidFile = "/tmp/hotify-cli.pid"
	logFile = "/tmp/hotify-cli.log"
)

// Exit codes for daemon operations
const (
	ExitDaemonError = 99
)

func startDaemon(port int) {
	format := getOutputFormat()

	// Check if already running
	if isDaemonRunning() {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     "Daemon is already running",
				Recoverable: false,
				Suggestions: []string{"Check daemon status with: hotify-cli status", "Stop daemon with: hotify-cli stop"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error getting executable path: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check executable permissions", "Verify hotify-cli is properly installed"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	// Create command to run server in foreground
	cmd := exec.Command(execPath, "start", fmt.Sprintf("-port=%d", port))

	// Set up logging
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error opening log file: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check log file permissions", "Ensure /tmp directory is writable"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}
	defer logFileHandle.Close()

	cmd.Stdout = logFileHandle
	cmd.Stderr = logFileHandle

	// Start the process
	if err := cmd.Start(); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error starting daemon: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check port availability", "Verify system resources"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	// Write PID file
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error writing PID file: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check /tmp directory permissions", "Ensure PID file location is writable"},
			},
		}
		printOutput(result, format)
		cmd.Process.Kill()
		os.Exit(ExitDaemonError)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"pid":      pid,
			"log_file": logFile,
			"port":     port,
			"status":   "started",
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func stopDaemon() {
	format := getOutputFormat()

	// Read PID file
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			result := CommandResult{
				Version: Version,
				Success: false,
				Error: &CommandError{
					Code:        ExitDaemonError,
					Type:        "daemon_error",
					Message:     "Daemon is not running",
					Recoverable: false,
					Suggestions: []string{"Start daemon with: hotify-cli start --daemon"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitDaemonError)
		}
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error reading PID file: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check PID file permissions", "Verify /tmp directory is accessible"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	// Send SIGTERM to the process
	process, err := os.FindProcess(pid)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error finding process: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check if process is still running", "Verify PID is correct"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitDaemonError,
				Type:        "daemon_error",
				Message:     fmt.Sprintf("Error stopping process: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check process permissions", "Verify process is still running"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitDaemonError)
	}

	// Remove PID file
	os.Remove(pidFile)

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"pid":    pid,
			"status": "stopped",
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func checkDaemonStatus() {
	format := getOutputFormat()

	if isDaemonRunning() {
		pidData, _ := os.ReadFile(pidFile)
		var pid int
		fmt.Sscanf(string(pidData), "%d", &pid)

		result := CommandResult{
			Version: Version,
			Success: true,
			Data: map[string]interface{}{
				"pid":      pid,
				"log_file": logFile,
				"status":   "running",
			},
			Metadata: map[string]interface{}{
				"timestamp": time.Now().Unix(),
			},
		}
		printOutput(result, format)
	} else {
		result := CommandResult{
			Version: Version,
			Success: true,
			Data: map[string]interface{}{
				"status": "not_running",
			},
			Metadata: map[string]interface{}{
				"timestamp": time.Now().Unix(),
			},
		}
		printOutput(result, format)
	}
}

func isDaemonRunning() bool {
	// Check if PID file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}

	// Read PID file
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process not running, clean up PID file
		os.Remove(pidFile)
		return false
	}

	return true
}
