package main

// Remote API handler for docker compose passthrough.
//
// Route registered via registerComposePassthroughRoute:
//   POST /api/remote/compose/exec
//
// Request body:
//   {
//     "app_id":      string (optional, resolves compose_path/compose_file)
//     "subcommand":  string  e.g. "up", "down", "restart", "ps", "logs"
//     "args":        []string (extra flags forwarded to docker compose)
//   }
//
// Response:
//   { "success": bool, "output": string, "exit_code": int, "warnings": []string }

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
)

func registerComposePassthroughRoute(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/compose/exec", authMiddleware(handleComposeExecRemoteAPI))
}

// handleComposeExecRemoteAPI executes docker compose on the server.
func handleComposeExecRemoteAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AppID      string   `json:"app_id"`
		Subcommand string   `json:"subcommand"`
		Args       []string `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Subcommand == "" {
		http.Error(w, "subcommand is required", http.StatusBadRequest)
		return
	}

	// Validate subcommand against allowlist to prevent arbitrary execution
	allowed := map[string]bool{
		"up": true, "down": true, "restart": true, "stop": true, "start": true,
		"ps": true, "logs": true, "pull": true, "build": true, "config": true,
		"exec": true, "run": true, "top": true, "events": true, "port": true,
		"images": true, "kill": true, "rm": true, "pause": true, "unpause": true,
		"wait": true, "cp": true, "ls": true, "version": true,
	}
	if !allowed[req.Subcommand] {
		http.Error(w, fmt.Sprintf("Subcommand '%s' not allowed", req.Subcommand), http.StatusForbidden)
		return
	}

	var workDir string
	var extraArgs []string
	warnings := []string{}

	if req.AppID != "" {
		config, err := loadConfig()
		if err != nil {
			http.Error(w, "Config load failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		app := findApp(config, req.AppID)
		if app == nil {
			http.Error(w, fmt.Sprintf("App '%s' not found", req.AppID), http.StatusNotFound)
			return
		}
		if app.ComposePath != "" {
			workDir = app.ComposePath
			if len(workDir) > 0 && workDir[0] == '~' {
				home := "/root"
				workDir = filepath.Join(home, workDir[1:])
			}
		} else {
			warnings = append(warnings, "App has no compose_path configured; running in default directory")
		}
		if app.ComposeFile != "" {
			extraArgs = append(extraArgs, "-f", app.ComposeFile)
		}
	}

	// Build docker compose args
	dockerArgs := append([]string{"compose"}, extraArgs...)
	dockerArgs = append(dockerArgs, req.Subcommand)
	dockerArgs = append(dockerArgs, req.Args...)

	cmd := exec.Command("docker", dockerArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	output := outBuf.String()
	if errBuf.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += errBuf.String()
	}

	auditLogger.LogEvent(AuditEvent{
		EventType: AuditEventDeploy,
		TokenName: r.Header.Get("X-API-Key-Name"),
		Details:   fmt.Sprintf("Compose exec: app=%s cmd=%s", req.AppID, strings.Join(dockerArgs, " ")),
		Success:   exitCode == 0,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   exitCode == 0,
		"output":    output,
		"exit_code": exitCode,
		"cmd":       strings.Join(dockerArgs, " "),
		"warnings":  warnings,
	})
}
