package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	pidFile = "/tmp/hotify-cli.pid"
	logFile = "/tmp/hotify-cli.log"
)

func startDaemon(port int) {
	// Check if already running
	if isDaemonRunning() {
		fmt.Println("Daemon is already running")
		return
	}

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	// Create command to run server in foreground
	cmd := exec.Command(execPath, "start", fmt.Sprintf("-port=%d", port))

	// Set up logging
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFileHandle.Close()

	cmd.Stdout = logFileHandle
	cmd.Stderr = logFileHandle

	// Start the process
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	// Write PID file
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing PID file: %v\n", err)
		cmd.Process.Kill()
		os.Exit(1)
	}

	fmt.Printf("Daemon started with PID %d\n", pid)
	fmt.Printf("Logs: %s\n", logFile)
}

func stopDaemon() {
	// Read PID file
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon is not running")
			return
		}
		fmt.Fprintf(os.Stderr, "Error reading PID file: %v\n", err)
		os.Exit(1)
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	// Send SIGTERM to the process
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding process: %v\n", err)
		os.Exit(1)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping process: %v\n", err)
		os.Exit(1)
	}

	// Remove PID file
	os.Remove(pidFile)

	fmt.Printf("Daemon stopped (PID %d)\n", pid)
}

func checkDaemonStatus() {
	if isDaemonRunning() {
		pidData, _ := os.ReadFile(pidFile)
		fmt.Printf("Daemon is running (PID %s)\n", string(pidData))
		fmt.Printf("Logs: %s\n", logFile)
	} else {
		fmt.Println("Daemon is not running")
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
