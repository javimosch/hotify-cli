package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── DeploymentClient extensions for compose ─────────────────────────────────

// DeployComposeFile sends a single file (compose YAML or .env) to the remote
// daemon, writing it to remotePath/filename.
func (d *DeploymentClient) DeployComposeFile(appID, localFile, remotePath string) error {
	data, err := os.ReadFile(localFile)
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}
	return d.HTTPClient.PostLarge("/api/deploy", map[string]interface{}{
		"app_id":      appID,
		"data":        base64.StdEncoding.EncodeToString(data),
		"data_type":   "binary",
		"target_path": filepath.Join(remotePath, filepath.Base(localFile)),
	})
}

// VolumeInit sends a tar.gz directory to populate a Docker named volume on the remote.
func (d *DeploymentClient) VolumeInit(appID, volumeName, sourcePath string) error {
	tarData, err := createTarGz(sourcePath)
	if err != nil {
		return fmt.Errorf("error creating tar.gz: %v", err)
	}
	return d.HTTPClient.PostLarge("/api/compose/volume-init", map[string]interface{}{
		"app_id":      appID,
		"volume_name": volumeName,
		"data":        base64.StdEncoding.EncodeToString(tarData),
	})
}

// PostLarge is like HTTPClient.Post but with a larger (5 min) timeout for big payloads.
func (h *HTTPClient) PostLarge(path string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}
	req, err := http.NewRequest("POST", h.BaseURL+path, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.AuthToken)
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	return nil
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// findApp returns a pointer to the App with the given ID, or nil.
func findApp(config *Config, appID string) *App {
	for i := range config.Apps {
		if config.Apps[i].ID == appID {
			return &config.Apps[i]
		}
	}
	return nil
}

// autoDetectComposeFile looks for common compose file names in dir.
func autoDetectComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml", "docker-compose.yaml",
		"compose.yml", "compose.yaml",
	}
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return ""
}

// exitConfigError prints a config error result and exits.
func exitConfigError(format OutputFormat, err error) {
	printOutput(CommandResult{
		Version: Version, Success: false,
		Error: &CommandError{Code: ExitConfigError, Type: "config_error", Message: err.Error(), Recoverable: false},
	}, format)
	os.Exit(ExitConfigError)
}

// exitAppNotFound prints a not-found result and exits.
func exitAppNotFound(format OutputFormat, appID string) {
	printOutput(CommandResult{
		Version: Version, Success: false,
		Error: &CommandError{
			Code: ExitInvalidArgument, Type: "not_found",
			Message:     fmt.Sprintf("App '%s' not found", appID),
			Recoverable: false,
			Suggestions: []string{"hotify-cli list"},
		},
	}, format)
	os.Exit(ExitInvalidArgument)
}

// exitTargetNotFound prints a target error result and exits.
func exitTargetNotFound(format OutputFormat, err error) {
	printOutput(CommandResult{
		Version: Version, Success: false,
		Error: &CommandError{
			Code: ExitTargetNotFound, Type: "target_error", Message: err.Error(),
			Recoverable: false,
			Suggestions: []string{"hotify-cli targets --action list"},
		},
	}, format)
	os.Exit(ExitTargetNotFound)
}

// exitClientError prints a client creation error result and exits.
func exitClientError(format OutputFormat, err error) {
	printOutput(CommandResult{
		Version: Version, Success: false,
		Error: &CommandError{
			Code: ExitGenericFailure, Type: "client_error", Message: err.Error(),
			Recoverable: false,
			Suggestions: []string{"Check target configuration", "Verify authentication token"},
		},
	}, format)
	os.Exit(ExitGenericFailure)
}
