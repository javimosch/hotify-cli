package main

import (
	"encoding/json"
	"flag"
	"fmt"
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

	req, err := http.NewRequest("POST", a.BaseURL+"/api/auth/login", nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.AuthToken)
	req.Body = newRequestBody(data)

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
		return 0, nil
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
	action := authCmd.String("action", "add", "Action: add, remove, list, test")
	authCmd.Parse(os.Args[2:])

	switch *action {
	case "add":
		handleAuthAdd(*url, *token, *name)
	case "remove":
		handleAuthRemove(*name)
	case "list":
		handleAuthList()
	case "test":
		handleAuthTest(*name)
	default:
		fmt.Println("Unknown action:", *action)
		fmt.Println("Valid actions: add, remove, list, test")
		os.Exit(1)
	}
}

func handleAuthAdd(url, token, name string) {
	if url == "" || token == "" || name == "" {
		fmt.Println("Missing required flags")
		fmt.Println("Usage: hotify-cli auth --url <url> --token <token> --name <name>")
		os.Exit(1)
	}

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Test connection first
	client, err := NewAuthClient(url, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating auth client: %v\n", err)
		os.Exit(1)
	}

	if err := client.Authenticate(); err != nil {
		fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
		os.Exit(1)
	}

	// Encrypt token and save to config
	security, err := NewSecurityManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating security manager: %v\n", err)
		os.Exit(1)
	}

	encryptedToken, err := security.EncryptToken(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encrypting token: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully authenticated with remote: %s\n", name)
	fmt.Printf("URL: %s\n", url)
	fmt.Printf("Permissions: %v\n", permissions)
}

func handleAuthRemove(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli auth remove --name <name>")
		os.Exit(1)
	}

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
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
		fmt.Printf("Remote '%s' not found\n", name)
		os.Exit(1)
	}

	config.Remotes = updatedRemotes

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Removed remote: %s\n", name)
}

func handleAuthList() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Remotes) == 0 {
		fmt.Println("No authenticated remotes")
		return
	}

	fmt.Println("Authenticated Remotes:")
	fmt.Println("====================")
	for _, remote := range config.Remotes {
		defaultMark := ""
		if remote.Default {
			defaultMark = " (default)"
		}
		fmt.Printf("Name: %s%s\n", remote.Name, defaultMark)
		fmt.Printf("URL: %s\n", remote.URL)
		fmt.Printf("Permissions: %v\n", remote.Permissions)
		fmt.Printf("Last Used: %s\n", remote.LastUsed)
		fmt.Println()
	}
}

func handleAuthTest(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli auth test --name <name>")
		os.Exit(1)
	}

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
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
		fmt.Printf("Remote '%s' not found\n", name)
		os.Exit(1)
	}

	// Decrypt token
	security, err := NewSecurityManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating security manager: %v\n", err)
		os.Exit(1)
	}

	token, err := security.DecryptToken(remote.AuthToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decrypting token: %v\n", err)
		os.Exit(1)
	}

	// Test connection
	client, err := NewAuthClient(remote.URL, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating auth client: %v\n", err)
		os.Exit(1)
	}

	valid, err := client.ValidateToken()
	if err != nil {
		fmt.Printf("❌ Connection test failed for %s: %v\n", name, err)
		os.Exit(1)
	}

	if !valid {
		fmt.Printf("❌ Connection test failed for %s: invalid token\n", name)
		os.Exit(1)
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

	fmt.Printf("✅ Connection successful to %s\n", name)
	fmt.Printf("Permissions: %v\n", remote.Permissions)
}
