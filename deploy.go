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

// Exit codes for deploy operations
const (
	ExitTraefikNotInstalled    = 90
	ExitTraefikAlreadyInstalled = 91
	ExitTraefikInstallFailed   = 92
	ExitTraefikServiceFailed   = 93
	ExitTraefikConfigInvalid   = 94
	ExitPermissionsError       = 95
	ExitTargetNotFound        = 96
	ExitConnectionTimeout     = 105
)

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
	
	// Filter out --human flag before parsing
	filteredArgs := filterHumanFlag(os.Args[2:])
	deployCmd.Parse(filteredArgs)

	// Determine output format (JSON by default, --human for text)
	format := getOutputFormat()

	if *appID == "" {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     "Missing required flag: --id",
				Recoverable: false,
				Suggestions: []string{"Provide app ID with --id flag"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Get target
	targetObj, err := getActiveTarget(*target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "target_error",
				Message:     err.Error(),
				Recoverable: false,
				Suggestions: []string{"Check target exists with: hotify-cli targets --action list", "Set default target with: hotify-cli targets --action use --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	switch *action {
	case "deploy":
		if *source == "" {
			result := CommandResult{
				Version: Version,
				Success: false,
				Error: &CommandError{
					Code:        ExitInvalidArgument,
					Type:        "validation_error",
					Message:     "Missing required flag: --source",
					Recoverable: false,
					Suggestions: []string{"Provide source path with --source flag"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
		}
		handleDeployAction(*appID, *source, targetObj, format)
	case "start":
		handleRemoteStart(*appID, targetObj, format)
	case "stop":
		handleRemoteStop(*appID, targetObj, format)
	case "restart":
		handleRemoteRestart(*appID, targetObj, format)
	case "status":
		handleRemoteStatus(*appID, targetObj, format)
	default:
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     fmt.Sprintf("Unknown action: %s", *action),
				Recoverable: false,
				Suggestions: []string{"Valid actions: deploy, start, stop, restart, status"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}
}

func handleDeployAction(appID, source string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Deploying app %s to target %s\n", appID, target.Name)
	}

	// Validate source
	if err := ValidateSource(source); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     fmt.Sprintf("Source validation failed: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check source path exists and is accessible", "Ensure source is a valid file or directory"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Check if source exists
	sourceInfo, err := os.Stat(source)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "validation_error",
				Message:     fmt.Sprintf("Error accessing source: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check source path is correct", "Ensure file/directory exists"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Create deployment client
	client, err := NewDeploymentClient(target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Determine deployment type based on source
	var deployErr error
	var targetPath string
	var deploymentType string

	if sourceInfo.IsDir() {
		// Folder deployment
		targetPath = fmt.Sprintf("/home/dk1/apps/%s", appID)
		deploymentType = "folder"

		if format == OutputFormatText {
			fmt.Printf("📦 Deploying folder: %s -> %s\n", source, targetPath)
		}
		deployErr = client.DeployFolder(appID, source, targetPath)
	} else {
		// Binary deployment
		targetPath = fmt.Sprintf("/home/dk1/apps/%s/%s", appID, filepath.Base(source))
		deploymentType = "binary"

		if format == OutputFormatText {
			fmt.Printf("🔧 Deploying binary: %s -> %s\n", source, targetPath)
		}
		deployErr = client.DeployBinary(appID, source, targetPath)
	}

	// Cleanup temporary files
	defer CleanupTempFiles()

	if deployErr != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "deployment_error",
				Message:     fmt.Sprintf("Deployment failed: %v", deployErr),
				Recoverable: true,
				Suggestions: []string{"Check network connectivity to target", "Verify target server is running", "Check target disk space"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":          appID,
			"target":          target.Name,
			"deployment_type": deploymentType,
			"source":          source,
			"target_path":     targetPath,
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func handleRemoteStart(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Starting app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.StartApp(appID); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "start_error",
				Message:     fmt.Sprintf("Failed to start app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check app configuration", "Verify app is installed", "Check app logs for errors"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":  appID,
			"target":  target.Name,
			"action":  "start",
			"status":  "started",
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func handleRemoteStop(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Stopping app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.StopApp(appID); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "stop_error",
				Message:     fmt.Sprintf("Failed to stop app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check if app is running", "Verify app configuration", "Check app logs for errors"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":  appID,
			"target":  target.Name,
			"action":  "stop",
			"status":  "stopped",
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func handleRemoteRestart(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Restarting app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	if err := client.RestartApp(appID); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "restart_error",
				Message:     fmt.Sprintf("Failed to restart app: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check app configuration", "Verify app is installed", "Check app logs for errors"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":  appID,
			"target":  target.Name,
			"action":  "restart",
			"status":  "restarted",
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}

func handleRemoteStatus(appID string, target *Remote, format OutputFormat) {
	if format == OutputFormatText {
		fmt.Printf("Checking status of app %s on target %s\n", appID, target.Name)
	}

	client, err := NewDeploymentClient(target)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "client_error",
				Message:     fmt.Sprintf("Error creating deployment client: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check target configuration", "Verify authentication token"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	status, err := client.GetAppStatus(appID)
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "status_error",
				Message:     fmt.Sprintf("Failed to get status: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check if app exists", "Verify app configuration", "Check target server connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"app_id":  appID,
			"target":  target.Name,
			"status":  status,
		},
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
		},
	}
	printOutput(result, format)
}
