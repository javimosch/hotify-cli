package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// findDeepestChild walks /proc to find the leaf-most child of ppid that is
// still alive. Returns ppid itself if no children are found.
// This resolves the PID of a daemon that forks off the shell process.
func findDeepestChild(ppid int) int {
	entries, err := ioutil.ReadDir("/proc")
	if err != nil {
		return ppid
	}

	// Build map: parent -> children
	children := map[int][]int{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		statPath := filepath.Join("/proc", e.Name(), "stat")
		data, err := ioutil.ReadFile(statPath)
		if err != nil {
			continue
		}
		// stat format: pid (comm) state ppid ...
		// find closing ')' to skip comm which may contain spaces
		s := string(data)
		idx := strings.LastIndex(s, ")")
		if idx < 0 {
			continue
		}
		fields := strings.Fields(s[idx+1:])
		if len(fields) < 2 {
			continue
		}
		parentPID, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		children[parentPID] = append(children[parentPID], pid)
	}

	// Walk down to the deepest single child
	current := ppid
	for {
		kids := children[current]
		if len(kids) == 0 {
			return current
		}
		if len(kids) == 1 {
			current = kids[0]
			continue
		}
		// Multiple children: pick the one with the highest PID (latest spawned)
		best := kids[0]
		for _, k := range kids[1:] {
			if k > best {
				best = k
			}
		}
		current = best
		break
	}
	return current
}

// resolveActualPID returns the real daemon PID after a fork chain.
// Strategy 1: walk /proc tree (works for single-fork processes).
// Strategy 2: port-based lookup via ss (works for double-fork/setsid daemons).
// Falls back to shellPID if neither resolves.
func resolveActualPID(shellPID int, port int) int {
	// Give the process time to fork and bind
	time.Sleep(300 * time.Millisecond)

	// Strategy 1: /proc tree walk (single-fork case)
	actual := findDeepestChild(shellPID)
	if actual != shellPID && pidExists(actual) {
		return actual
	}

	// Strategy 2: port-based lookup (double-fork/setsid daemons like cmdcenter)
	if port > 0 {
		if pid := findPIDByPort(port); pid > 0 {
			return pid
		}
	}

	// Fallback: return shell PID and let the caller deal with it
	return shellPID
}

// findPIDByPort uses ss to find the PID listening on a given TCP port
func findPIDByPort(port int) int {
	out, err := exec.Command("ss", "-tlnp", fmt.Sprintf("sport = :%d", port)).Output()
	if err != nil {
		return 0
	}
	// Parse: users:(("name",pid=12345,fd=3))
	s := string(out)
	pidMarker := "pid="
	idx := strings.Index(s, pidMarker)
	if idx < 0 {
		return 0
	}
	rest := s[idx+len(pidMarker):]
	end := strings.IndexAny(rest, ",)")
	if end < 0 {
		return 0
	}
	pid, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return pid
}

// pidExists checks whether a given PID is alive
func pidExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// readPIDFile tries to read a PID from a .pid file written by the app.
// Convention: /tmp/<appID>.pid
func readPIDFile(appID string) (int, error) {
	data, err := ioutil.ReadFile(fmt.Sprintf("/tmp/%s.pid", appID))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

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

	// Resolve actual daemon PID: the shell may fork the real process
	// and exit immediately. Use /proc tree walk then port-based fallback.
	actualPID := resolveActualPID(cmd.Process.Pid, app.Port)

	// Update config with resolved PID and status
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			config.Apps[i].PID = actualPID
			config.Apps[i].Status = "running"
			saveConfig(config)
			break
		}
	}

	printOutput(CommandResult{
		Version: Version, Success: true,
		Data:    map[string]interface{}{"app_id": appID, "status": "running", "pid": actualPID, "shell_pid": cmd.Process.Pid},
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