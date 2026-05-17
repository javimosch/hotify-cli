package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// AuthClient handles authentication with remote daemon
type AuthClient struct {
	BaseURL    string
	AuthToken  string
	HTTPClient *http.Client
	Security   *SecurityManager
}

// NewAuthClient creates a new authentication client
func NewAuthClient(baseURL, authToken string) (*AuthClient, error) {
	security, err := NewSecurityManager()
	if err != nil {
		return nil, err
	}

	return &AuthClient{
		BaseURL: baseURL,
		AuthToken: authToken,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Security: security,
	}, nil
}

// Authenticate authenticates with the remote daemon
func (a *AuthClient) Authenticate() error {
	payload := map[string]string{
		"token": a.AuthToken,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}

	req, err := http.NewRequest("POST", a.BaseURL+"/api/auth/login", newRequestBody(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with status %d", resp.StatusCode)
	}

	return nil
}

// ValidateToken validates the current authentication token
func (a *AuthClient) ValidateToken() (bool, error) {
	req, err := http.NewRequest("GET", a.BaseURL+"/api/auth/validate", nil)
	if err != nil {
		return false, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.AuthToken)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// RefreshToken refreshes the authentication token
func (a *AuthClient) RefreshToken() error {
	req, err := http.NewRequest("POST", a.BaseURL+"/api/auth/refresh", nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.AuthToken)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	// Parse new token from response
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("error parsing response: %v", err)
	}

	newToken, exists := result["token"]
	if !exists {
		return fmt.Errorf("no token in response")
	}

	a.AuthToken = newToken
	return nil
}

// Logout invalidates the current token
func (a *AuthClient) Logout() error {
	req, err := http.NewRequest("POST", a.BaseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.AuthToken)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("logout failed with status %d", resp.StatusCode)
	}

	return nil
}

// HasPermission checks if the current token has a specific permission
func (a *AuthClient) HasPermission(permission string) (bool, error) {
	req, err := http.NewRequest("GET", a.BaseURL+"/api/auth/permissions?perm="+permission, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+a.AuthToken)

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("error parsing response: %v", err)
	}

	return result["has_permission"], nil
}

// newRequestBody creates a request body from byte data
func newRequestBody(data []byte) *requestBody {
	return &requestBody{data: data}
}

type requestBody struct {
	data []byte
	pos  int
}

func (r *requestBody) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *requestBody) Close() error {
	return nil
}

// handleAuth handles the auth CLI command
func handleAuth() {
	authCmd := flag.NewFlagSet("auth", flag.ExitOnError)
	url := authCmd.String("url", "", "Remote daemon URL")
	token := authCmd.String("token", "", "Authentication token")
	name := authCmd.String("name", "", "Remote name")
	target := authCmd.String("target", "", "Target name (uses default if not specified)")
	action := authCmd.String("action", "add", "Action: add, remove, list, test")
	
	// Filter out --human flag before parsing
	filteredArgs := filterHumanFlag(os.Args[2:])
	authCmd.Parse(filteredArgs)

	// Determine output format (JSON by default, --human for text)
	format := getOutputFormat()

	switch *action {
	case "add":
		handleAuthAdd(*url, *token, *name, format)
	case "remove":
		handleAuthRemove(*name, format)
	case "list":
		handleAuthList(format)
	case "test":
		// Use --target if specified, otherwise use --name
		if *target != "" {
			handleAuthTest(*target, format)
		} else {
			handleAuthTest(*name, format)
		}
	default:
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "invalid_argument",
				Message:     fmt.Sprintf("Unknown action: %s", *action),
				Recoverable: false,
				Suggestions: []string{"Valid actions: add, remove, list, test"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}
}

func handleAuthAdd(url, token, name string, format OutputFormat) {
	if url == "" || token == "" || name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flags: url, token, name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli auth --action add --url <url> --token <token> --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Test connection first
	client, err := NewAuthClient(url, token)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "connection_failed",
				Message:     fmt.Sprintf("Error creating auth client: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check URL is correct", "Check network connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	if err := client.Authenticate(); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "authentication_failed",
				Message:     fmt.Sprintf("Authentication failed: %v", err),
				Recoverable: true,
				Suggestions: []string{"Verify token is correct", "Check remote daemon is running"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	// Encrypt token and save to config
	security, err := NewSecurityManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "security_manager_failed",
				Message:     fmt.Sprintf("Error creating security manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	encryptedToken, err := security.EncryptToken(token)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "encryption_failed",
				Message:     fmt.Sprintf("Error encrypting token: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Get permissions
	permissions := []string{"deploy", "start", "stop", "logs"} // Default permissions

	// Add remote to config
	newRemote := Remote{
		Name:        name,
		URL:         url,
		AuthToken:   encryptedToken,
		Permissions: permissions,
		Default:     len(config.Remotes) == 0,
		LastUsed:    time.Now().Format(time.RFC3339),
	}

	config.Remotes = append(config.Remotes, newRemote)

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_save_failed",
				Message:     fmt.Sprintf("Error saving config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"remote": map[string]interface{}{
				"name":        name,
				"url":         url,
				"permissions": permissions,
				"default":     len(config.Remotes) == 1,
			},
		},
		Metadata: map[string]interface{}{
			"action": "authenticated",
		},
	}
	printOutput(result, format)
}

func handleAuthRemove(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli auth --action remove --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find and remove remote
	found := false
	var updatedRemotes []Remote
	for _, remote := range config.Remotes {
		if remote.Name != name {
			updatedRemotes = append(updatedRemotes, remote)
		} else {
			found = true
		}
	}

	if !found {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "remote_not_found",
				Message:     fmt.Sprintf("Remote '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"List available remotes: hotify-cli auth --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	config.Remotes = updatedRemotes

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_save_failed",
				Message:     fmt.Sprintf("Error saving config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"removed_remote": name,
		},
		Metadata: map[string]interface{}{
			"action": "removed",
		},
	}
	printOutput(result, format)
}

func handleAuthList(format OutputFormat) {
	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	remotes := []map[string]interface{}{}
	for _, remote := range config.Remotes {
		remotes = append(remotes, map[string]interface{}{
			"name":        remote.Name,
			"url":         remote.URL,
			"permissions": remote.Permissions,
			"default":     remote.Default,
			"last_used":   remote.LastUsed,
		})
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"remotes": remotes,
			"count":   len(remotes),
		},
	}
	printOutput(result, format)
}

func handleAuthTest(name string, format OutputFormat) {
	// If no name specified, use default target
	if name == "" {
		target, err := getActiveTarget("")
		if err != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitTargetNotFound,
					Type:        "no_default_target",
					Message:     fmt.Sprintf("Error: %v", err),
					Recoverable: false,
					Suggestions: []string{
						"Usage: hotify-cli auth --action test --name <name>",
						"Set default target: hotify-cli targets --action use --name <name>",
					},
				},
			}
			printOutput(result, format)
			os.Exit(ExitTargetNotFound)
		}
		name = target.Name
	}

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "config_load_failed",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Find remote
	var remote *Remote
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			remote = &config.Remotes[i]
			break
		}
	}

	if remote == nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "remote_not_found",
				Message:     fmt.Sprintf("Remote '%s' not found", name),
				Recoverable: false,
				Suggestions: []string{"List available remotes: hotify-cli auth --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	// Decrypt token
	security, err := NewSecurityManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "security_manager_failed",
				Message:     fmt.Sprintf("Error creating security manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	token, err := security.DecryptToken(remote.AuthToken)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "decryption_failed",
				Message:     fmt.Sprintf("Error decrypting token: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	// Test connection
	client, err := NewAuthClient(remote.URL, token)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "connection_failed",
				Message:     fmt.Sprintf("Error creating auth client: %v", err),
				Recoverable: true,
				Suggestions: []string{"Check URL is correct", "Check network connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	valid, err := client.ValidateToken()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "connection_test_failed",
				Message:     fmt.Sprintf("Connection test failed for %s: %v", name, err),
				Recoverable: true,
				Suggestions: []string{"Check remote daemon is running", "Verify network connectivity"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	if !valid {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitConnectionTimeout,
				Type:        "invalid_token",
				Message:     fmt.Sprintf("Connection test failed for %s: invalid token", name),
				Recoverable: false,
				Suggestions: []string{"Re-authenticate: hotify-cli auth --action add --url <url> --token <token> --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitConnectionTimeout)
	}

	// Update last used
	now := time.Now().Format(time.RFC3339)
	for i := range config.Remotes {
		if config.Remotes[i].Name == name {
			config.Remotes[i].LastUsed = now
			break
		}
	}
	saveConfig(config)

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"remote": map[string]interface{}{
				"name":        name,
				"url":         remote.URL,
				"permissions": remote.Permissions,
				"valid":       true,
			},
		},
		Metadata: map[string]interface{}{
			"action": "tested",
		},
	}
	printOutput(result, format)
}
