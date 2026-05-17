package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"archive/tar"
	"compress/gzip"
)

// DeploymentClient handles deployment operations
type DeploymentClient struct {
	Target    *Remote
	HTTPClient *HTTPClient
}

// NewDeploymentClient creates a new deployment client
func NewDeploymentClient(target *Remote) (*DeploymentClient, error) {
	security, err := NewSecurityManager()
	if err != nil {
		return nil, err
	}

	token, err := security.DecryptToken(target.AuthToken)
	if err != nil {
		return nil, err
	}

	return &DeploymentClient{
		Target: target,
		HTTPClient: &HTTPClient{
			BaseURL:   target.URL,
			AuthToken: token,
		},
	}, nil
}

// DeployBinary deploys a binary to the remote server
func (d *DeploymentClient) DeployBinary(appID string, sourcePath string, targetPath string) error {
	// Read binary file
	binaryData, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("error reading binary file: %v", err)
	}

	// Encode to base64
	encodedBinary := base64.StdEncoding.EncodeToString(binaryData)

	// Send deployment request
	payload := map[string]interface{}{
		"app_id":       appID,
		"data":         encodedBinary,
		"data_type":    "binary",
		"target_path":  targetPath,
	}

	return d.HTTPClient.Post("/api/deploy", payload)
}

// DeployFolder deploys a folder to the remote server
func (d *DeploymentClient) DeployFolder(appID string, folderPath string, targetPath string) error {
	// Create tar.gz from folder
	tarData, err := createTarGz(folderPath)
	if err != nil {
		return fmt.Errorf("error creating tar.gz: %v", err)
	}

	// Encode to base64
	encodedData := base64.StdEncoding.EncodeToString(tarData)

	// Send deployment request
	payload := map[string]interface{}{
		"app_id":       appID,
		"data":         encodedData,
		"data_type":    "folder",
		"target_path":  targetPath,
	}

	return d.HTTPClient.Post("/api/deploy", payload)
}

// createTarGz creates a tar.gz archive from a folder
func createTarGz(sourcePath string) ([]byte, error) {
	var buf bytes.Buffer

	// Create gzip writer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Walk through the directory
	err := filepath.Walk(sourcePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Update header name to be relative to source path
		relPath, err := filepath.Rel(sourcePath, filePath)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content for regular files
		if !info.IsDir() {
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Close writers to flush data
	tarWriter.Close()
	gzWriter.Close()

	return buf.Bytes(), nil
}

// DeployBinaryRsync deploys a binary using rsync
func (d *DeploymentClient) DeployBinaryRsync(sourcePath, remotePath string) error {
	// Extract host from target URL
	url := d.Target.URL
	host := strings.TrimPrefix(url, "http://")
	host = strings.Split(host, ":")[0]

	// Build rsync command
	rsyncCmd := exec.Command("rsync", "-avz", "--progress", sourcePath, host+":"+remotePath)

	// Run rsync
	var stdout, stderr bytes.Buffer
	rsyncCmd.Stdout = &stdout
	rsyncCmd.Stderr = &stderr

	if err := rsyncCmd.Run(); err != nil {
		return fmt.Errorf("rsync failed: %v\nStdout: %s\nStderr: %s", err, stdout.String(), stderr.String())
	}

	fmt.Printf("✅ Rsync completed: %s -> %s:%s\n", sourcePath, host, remotePath)
	return nil
}

// StartApp starts an application on the remote server
func (d *DeploymentClient) StartApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/start", appID), nil)
}

// StopApp stops an application on the remote server
func (d *DeploymentClient) StopApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/stop", appID), nil)
}

// RestartApp restarts an application on the remote server
func (d *DeploymentClient) RestartApp(appID string) error {
	return d.HTTPClient.Post(fmt.Sprintf("/api/apps/%s/restart", appID), nil)
}

// GetAppStatus gets the status of an application
func (d *DeploymentClient) GetAppStatus(appID string) (map[string]interface{}, error) {
	return d.HTTPClient.Get(fmt.Sprintf("/api/apps/%s/status", appID))
}

// GetAppLogs gets the logs of an application
func (d *DeploymentClient) GetAppLogs(appID string) (map[string]interface{}, error) {
	return d.HTTPClient.Get(fmt.Sprintf("/api/apps/%s/logs", appID))
}

// HTTPClient is a simple HTTP client for API calls
type HTTPClient struct {
	BaseURL   string
	AuthToken string
}

func (h *HTTPClient) Post(path string, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}

	req, err := http.NewRequest("POST", h.BaseURL+path, newRequestBody(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.AuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return nil
}

// OutputFormat controls command output format
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

// CommandResult represents a structured command result
type CommandResult struct {
	Version  string                 `json:"version"`
	Success  bool                   `json:"success"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    *CommandError          `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CommandError represents a structured error
type CommandError struct {
	Code        int      `json:"code"`
	Type        string   `json:"type"`
	Message     string   `json:"message"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Recoverable bool     `json:"recoverable"`
	RetryAfter  *int     `json:"retry_after,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Exit codes
const (
	ExitSuccess                = 0
	ExitGenericFailure         = 1
	ExitTraefikNotInstalled    = 90
	ExitTraefikAlreadyInstalled = 91
	ExitTraefikInstallFailed   = 92
	ExitTraefikServiceFailed   = 93
	ExitTraefikConfigInvalid   = 94
	ExitPermissionsError       = 95
	ExitTargetNotFound        = 96
	ExitInvalidArgument       = 97
	ExitConnectionTimeout     = 105
)

// printOutput prints command result in the specified format
func printOutput(result CommandResult, format OutputFormat) {
	if format == OutputFormatJSON {
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if result.Success {
			fmt.Println("✅ Success")
			for key, value := range result.Data {
				fmt.Printf("%s: %v\n", key, value)
			}
		} else {
			fmt.Fprintf(os.Stderr, "❌ Error: %s\n", result.Error.Message)
			for _, suggestion := range result.Error.Suggestions {
				fmt.Fprintf(os.Stderr, "  → %s\n", suggestion)
			}
		}
	}
}

func (h *HTTPClient) Get(path string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", h.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+h.AuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return result, nil
}

// LocalDeploy handles local deployment operations
func LocalDeploy(sourcePath, remotePath string) error {
	// Ensure remote directory exists
	if err := os.MkdirAll(filepath.Dir(remotePath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	// Copy file
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("error opening source file: %v", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(remotePath)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("error copying file: %v", err)
	}

	// Set executable permissions
	if err := os.Chmod(remotePath, 0755); err != nil {
		return fmt.Errorf("error setting permissions: %v", err)
	}

	fmt.Printf("✅ Deployed: %s -> %s\n", sourcePath, remotePath)
	return nil
}

// ValidateDeployment checks if a deployment was successful
func ValidateDeployment(binaryPath string) error {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at: %s", binaryPath)
	}

	// Check if file is executable
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("error checking file: %v", err)
	}

	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", binaryPath)
	}

	return nil
}

// ValidateSource validates the source path before deployment
func ValidateSource(sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("error accessing source: %v", err)
	}

	if info.IsDir() {
		// Check if directory is not empty
		entries, err := os.ReadDir(sourcePath)
		if err != nil {
			return fmt.Errorf("error reading directory: %v", err)
		}
		if len(entries) == 0 {
			return fmt.Errorf("source directory is empty: %s", sourcePath)
		}
	} else {
		// Check if file is readable
		file, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("error opening file: %v", err)
		}
		file.Close()
	}

	return nil
}

// CleanupTempFiles removes temporary deployment files
func CleanupTempFiles() {
	tmpFiles := []string{
		"/tmp/deploy_*.tar.gz",
	}

	for _, pattern := range tmpFiles {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			os.Remove(match)
		}
	}
}

// handleDeploy handles the deploy CLI command
func handleDeploy() {
	deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
	appID := deployCmd.String("id", "", "App ID (required)")
	source := deployCmd.String("source", "", "Source binary path (required)")
	target := deployCmd.String("target", "", "Target name (uses default if not specified)")
	action := deployCmd.String("action", "deploy", "Action: deploy, start, stop, restart, status")
	deployCmd.Parse(os.Args[2:])

	if *appID == "" {
		fmt.Println("Missing required flag: --id")
		fmt.Println("Usage: hotify-cli deploy --id <id> --source <source> [--target <target>]")
		os.Exit(1)
	}

	// Get target
	targetObj, err := getActiveTarget(*target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch *action {
	case "deploy":
		if *source == "" {
			fmt.Println("Missing required flag: --source")
			fmt.Println("Usage: hotify-cli deploy --id <id> --source <source>")
			os.Exit(1)
		}
		handleDeployAction(*appID, *source, targetObj)
	case "start":
		handleRemoteStart(*appID, targetObj)
	case "stop":
		handleRemoteStop(*appID, targetObj)
	case "restart":
		handleRemoteRestart(*appID, targetObj)
	case "status":
		handleRemoteStatus(*appID, targetObj)
	default:
		fmt.Println("Unknown action:", *action)
		fmt.Println("Valid actions: deploy, start, stop, restart, status")
		os.Exit(1)
	}
}

func handleDeployAction(appID, source string, target *Remote) {
	fmt.Printf("Deploying app %s to target %s\n", appID, target.Name)

	// Validate source
	if err := ValidateSource(source); err != nil {
		fmt.Fprintf(os.Stderr, "Source validation failed: %v\n", err)
		os.Exit(1)
	}

	// Check if source exists
	sourceInfo, err := os.Stat(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing source: %v\n", err)
		os.Exit(1)
	}

	// Create deployment client
	client, err := NewDeploymentClient(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating deployment client: %v\n", err)
		os.Exit(1)
	}

	// Determine deployment type based on source
	var deployErr error
	var targetPath string
	
	if sourceInfo.IsDir() {
		// Folder deployment
		targetPath = fmt.Sprintf("/home/dk1/apps/%s", appID)
		
		fmt.Printf("📦 Deploying folder: %s -> %s\n", source, targetPath)
		deployErr = client.DeployFolder(appID, source, targetPath)
	} else {
		// Binary deployment
		targetPath = fmt.Sprintf("/home/dk1/apps/%s/%s", appID, filepath.Base(source))
		
		fmt.Printf("🔧 Deploying binary: %s -> %s\n", source, targetPath)
		deployErr = client.DeployBinary(appID, source, targetPath)
	}

	// Cleanup temporary files
	defer CleanupTempFiles()

	if deployErr != nil {
		fmt.Fprintf(os.Stderr, "Deployment failed: %v\n", deployErr)
		os.Exit(1)
	}

	fmt.Printf("✅ Deployment successful: %s\n", appID)
}

func handleRemoteStart(appID string, target *Remote) {
	fmt.Printf("Starting app %s on target %s\n", appID, target.Name)

	client, err := NewDeploymentClient(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating deployment client: %v\n", err)
		os.Exit(1)
	}

	if err := client.StartApp(appID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start app: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ App started: %s\n", appID)
}

func handleRemoteStop(appID string, target *Remote) {
	fmt.Printf("Stopping app %s on target %s\n", appID, target.Name)

	client, err := NewDeploymentClient(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating deployment client: %v\n", err)
		os.Exit(1)
	}

	if err := client.StopApp(appID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stop app: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ App stopped: %s\n", appID)
}

func handleRemoteRestart(appID string, target *Remote) {
	fmt.Printf("Restarting app %s on target %s\n", appID, target.Name)

	client, err := NewDeploymentClient(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating deployment client: %v\n", err)
		os.Exit(1)
	}

	if err := client.RestartApp(appID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to restart app: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ App restarted: %s\n", appID)
}

func handleRemoteStatus(appID string, target *Remote) {
	fmt.Printf("Checking status of app %s on target %s\n", appID, target.Name)

	client, err := NewDeploymentClient(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating deployment client: %v\n", err)
		os.Exit(1)
	}

	status, err := client.GetAppStatus(appID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Status: %v\n", status)
}
