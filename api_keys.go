package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// APIKey represents an API key for authentication
type APIKey struct {
	Name        string      `json:"name"`
	Token       string      `json:"token"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time   `json:"created_at"`
	ExpiresAt   time.Time   `json:"expires_at,omitempty"`
	LastUsed    time.Time   `json:"last_used"`
	RequestCount int        `json:"request_count"`
	FailedAttempts int      `json:"failed_attempts"`
}

// APIKeyManager manages API keys
type APIKeyManager struct {
	keys       map[string]*APIKey
	permMgr    *PermissionManager
	audit      *AuditLogger
	security   *SecurityManager
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager() (*APIKeyManager, error) {
	security, err := NewSecurityManager()
	if err != nil {
		return nil, err
	}

	audit, err := NewAuditLogger()
	if err != nil {
		return nil, err
	}

	// Load existing audit events
	audit.LoadEventsFromDisk()

	return &APIKeyManager{
		keys:    make(map[string]*APIKey),
		permMgr: NewPermissionManager(),
		audit:   audit,
		security: security,
	}, nil
}

// AddKey creates a new API key
func (a *APIKeyManager) AddKey(name string, permissions []Permission, token string) (*APIKey, error) {
	if _, exists := a.keys[name]; exists {
		return nil, fmt.Errorf("API key with name '%s' already exists", name)
	}

	// Generate token if not provided
	if token == "" {
		var err error
		token, err = a.security.GenerateToken()
		if err != nil {
			return nil, fmt.Errorf("error generating token: %v", err)
		}
	}

	// Validate permissions
	for _, perm := range permissions {
		if !ValidatePermission(string(perm)) {
			return nil, fmt.Errorf("invalid permission: %s", perm)
		}
	}

	// If admin is in permissions, grant all permissions
	hasAdmin := false
	for _, perm := range permissions {
		if perm == PermissionAdmin {
			hasAdmin = true
			break
		}
	}

	finalPermissions := permissions
	if hasAdmin {
		finalPermissions = AllPermissions
	}

	key := &APIKey{
		Name:          name,
		Token:         token,
		Permissions:   finalPermissions,
		CreatedAt:     time.Now(),
		LastUsed:      time.Now(),
		RequestCount:  0,
		FailedAttempts: 0,
	}

	// Set expiration if configured
	config, _ := loadConfig()
	if config.Security.TokenExpirationDays > 0 {
		key.ExpiresAt = time.Now().AddDate(0, 0, config.Security.TokenExpirationDays)
	}

	a.keys[name] = key
	a.permMgr.apiKeys = a.keys

	// Log event
	a.audit.LogEvent(AuditEvent{
		EventType: AuditEventAPIKeyCreate,
		TokenName: name,
		Details:  fmt.Sprintf("Created API key with permissions: %v", finalPermissions),
		Success:  true,
	})

	return key, nil
}

// RemoveKey removes an API key
func (a *APIKeyManager) RemoveKey(name string) error {
	key, exists := a.keys[name]
	if !exists {
		return fmt.Errorf("API key '%s' not found", name)
	}

	delete(a.keys, name)
	delete(a.permMgr.apiKeys, key.Token)

	// Log event
	a.audit.LogEvent(AuditEvent{
		EventType: AuditEventAPIKeyDelete,
		TokenName: name,
		Details:  "API key removed",
		Success:  true,
	})

	return nil
}

// RegenerateKey regenerates an API key's token
func (a *APIKeyManager) RegenerateKey(name string) (*APIKey, error) {
	key, exists := a.keys[name]
	if !exists {
		return nil, fmt.Errorf("API key '%s' not found", name)
	}

	// Generate new token
	newToken, err := a.security.GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("error generating token: %v", err)
	}

	// Update key
	oldToken := key.Token
	key.Token = newToken
	key.LastUsed = time.Now()

	// Update permission manager
	delete(a.permMgr.apiKeys, oldToken)
	a.permMgr.apiKeys[newToken] = key

	// Log event
	a.audit.LogEvent(AuditEvent{
		EventType: AuditEventAPIKeyRegen,
		TokenName: name,
		Details:  "API key token regenerated",
		Success:  true,
	})

	return key, nil
}

// ListKeys returns all API keys
func (a *APIKeyManager) ListKeys() []*APIKey {
	var keys []*APIKey
	for _, key := range a.keys {
		keys = append(keys, key)
	}
	return keys
}

// GetKey returns an API key by name
func (a *APIKeyManager) GetKey(name string) (*APIKey, error) {
	key, exists := a.keys[name]
	if !exists {
		return nil, fmt.Errorf("API key '%s' not found", name)
	}
	return key, nil
}

// GetKeyByToken returns an API key by token
func (a *APIKeyManager) GetKeyByToken(token string) (*APIKey, error) {
	for _, key := range a.keys {
		if key.Token == token {
			return key, nil
		}
	}
	return nil, fmt.Errorf("API key not found for token")
}

// UpdatePermissions updates an API key's permissions
func (a *APIKeyManager) UpdatePermissions(name string, add, remove []Permission) error {
	key, exists := a.keys[name]
	if !exists {
		return fmt.Errorf("API key '%s' not found", name)
	}

	// Add permissions
	for _, perm := range add {
		if !a.permMgr.CheckPermission(key.Token, perm) {
			if err := a.permMgr.GrantPermission(key.Token, perm); err != nil {
				return err
			}
		}
	}

	// Remove permissions
	for _, perm := range remove {
		if err := a.permMgr.RevokePermission(key.Token, perm); err != nil {
			return err
		}
	}

	// Log changes
	a.audit.LogEvent(AuditEvent{
		EventType: AuditEventPermissionAdd,
		TokenName: name,
		Details:  fmt.Sprintf("Updated permissions - add: %v, remove: %v", add, remove),
		Success:  true,
	})

	return nil
}

// ValidateKey validates an API key and returns it if valid
func (a *APIKeyManager) ValidateKey(token string) (*APIKey, error) {
	key, err := a.GetKeyByToken(token)
	if err != nil {
		// Log failed attempt
		a.audit.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			Details:   "Invalid API token",
			Success:   false,
		})
		return nil, err
	}

	// Check expiration
	if !key.ExpiresAt.IsZero() && time.Now().After(key.ExpiresAt) {
		a.audit.LogEvent(AuditEvent{
			EventType: AuditEventAuthFailed,
			TokenName: key.Name,
			Details:   "API key expired",
			Success:   false,
		})
		return nil, fmt.Errorf("API key '%s' has expired", key.Name)
	}

	// Update last used and request count
	key.LastUsed = time.Now()
	key.RequestCount++

	return key, nil
}

// RecordFailedAttempt records a failed authentication attempt
func (a *APIKeyManager) RecordFailedAttempt(token string) error {
	key, err := a.GetKeyByToken(token)
	if err != nil {
		return err
	}

	key.FailedAttempts++

	// Check if max attempts exceeded
	config, _ := loadConfig()
	if config.Security.MaxFailedAttempts > 0 && 
	   key.FailedAttempts >= config.Security.MaxFailedAttempts {
		// Disable key
		return fmt.Errorf("max failed attempts exceeded for key '%s'", key.Name)
	}

	return nil
}

// GetKeyUsage returns usage statistics for a key
func (a *APIKeyManager) GetKeyUsage(name string) (map[string]interface{}, error) {
	key, err := a.GetKey(name)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"last_used":        key.LastUsed.Format(time.RFC3339),
		"request_count":    key.RequestCount,
		"failed_attempts":  key.FailedAttempts,
		"created_at":       key.CreatedAt.Format(time.RFC3339),
		"expires_at":       key.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// handleAPIKeys handles the api-keys CLI command
func handleAPIKeys() {
	apiKeysCmd := flag.NewFlagSet("api-keys", flag.ExitOnError)
	action := apiKeysCmd.String("action", "list", "Action: add, list, remove, regenerate, permissions, usage")
	name := apiKeysCmd.String("name", "", "API key name")
	token := apiKeysCmd.String("token", "auto", "API token (auto to generate)")
	permissions := apiKeysCmd.String("permissions", "", "Comma-separated permissions")
	addPerms := apiKeysCmd.String("add", "", "Permissions to add")
	removePerms := apiKeysCmd.String("remove", "", "Permissions to remove")
	apiKeysCmd.Parse(os.Args[2:])

	switch *action {
	case "add":
		handleAPIKeyAdd(*name, *token, *permissions)
	case "list":
		handleAPIKeyList()
	case "remove":
		handleAPIKeyRemove(*name)
	case "regenerate":
		handleAPIKeyRegenerate(*name)
	case "permissions":
		handleAPIKeyPermissions(*name, *addPerms, *removePerms)
	case "usage":
		handleAPIKeyUsage(*name)
	default:
		fmt.Println("Unknown action:", *action)
		fmt.Println("Valid actions: add, list, remove, regenerate, permissions, usage")
		os.Exit(1)
	}
}

func handleAPIKeyAdd(name, token, permissions string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli api-keys add --name <name> [--token <token>] [--permissions <permissions>]")
		os.Exit(1)
	}

	// Parse permissions
	var perms []Permission
	var err error
	if permissions != "" {
		perms, err = ParsePermissions(permissions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing permissions: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Default permissions
		perms = []Permission{PermissionDeploy, PermissionStart, PermissionStop, PermissionLogs}
	}

	// Handle auto token
	tokenValue := token
	if token == "auto" {
		tokenValue = ""
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	key, err := manager.AddKey(name, perms, tokenValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ API key created: %s\n", key.Name)
	fmt.Printf("Token: %s\n", key.Token)
	fmt.Printf("Permissions: %v\n", key.Permissions)
	if !key.ExpiresAt.IsZero() {
		fmt.Printf("Expires: %s\n", key.ExpiresAt.Format(time.RFC3339))
	}
	fmt.Println("\nIMPORTANT: Store this token securely. It will not be shown again.")
}

func handleAPIKeyList() {
	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	keys := manager.ListKeys()
	if len(keys) == 0 {
		fmt.Println("No API keys configured")
		return
	}

	fmt.Println("API Keys:")
	fmt.Println("=========")
	for _, key := range keys {
		fmt.Printf("Name: %s\n", key.Name)
		fmt.Printf("Token: %s\n", maskToken(key.Token))
		fmt.Printf("Permissions: %v\n", key.Permissions)
		fmt.Printf("Created: %s\n", key.CreatedAt.Format(time.RFC3339))
		if !key.ExpiresAt.IsZero() {
			fmt.Printf("Expires: %s\n", key.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Printf("Last Used: %s\n", key.LastUsed.Format(time.RFC3339))
		fmt.Printf("Requests: %d\n", key.RequestCount)
		fmt.Printf("Failed Attempts: %d\n", key.FailedAttempts)
		fmt.Println()
	}
}

func handleAPIKeyRemove(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli api-keys remove --name <name>")
		os.Exit(1)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	if err := manager.RemoveKey(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ API key removed: %s\n", name)
}

func handleAPIKeyRegenerate(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli api-keys regenerate --name <name>")
		os.Exit(1)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	key, err := manager.RegenerateKey(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error regenerating API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ API key regenerated: %s\n", key.Name)
	fmt.Printf("New Token: %s\n", key.Token)
	fmt.Println("\nIMPORTANT: Update your authentication with this new token.")
}

func handleAPIKeyPermissions(name, add, remove string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli api-keys permissions --name <name> [--add <perms>] [--remove <perms>]")
		os.Exit(1)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	var addPerms, removePerms []Permission
	var err1, err2 error

	if add != "" {
		addPerms, err1 = ParsePermissions(add)
		if err1 != nil {
			fmt.Fprintf(os.Stderr, "Error parsing add permissions: %v\n", err1)
			os.Exit(1)
		}
	}

	if remove != "" {
		removePerms, err2 = ParsePermissions(remove)
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "Error parsing remove permissions: %v\n", err2)
			os.Exit(1)
		}
	}

	if err := manager.UpdatePermissions(name, addPerms, removePerms); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating permissions: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Permissions updated for: %s\n", name)
}

func handleAPIKeyUsage(name string) {
	if name == "" {
		fmt.Println("Missing required flag: --name")
		fmt.Println("Usage: hotify-cli api-keys usage --name <name>")
		os.Exit(1)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key manager: %v\n", err)
		os.Exit(1)
	}

	usage, err := manager.GetKeyUsage(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting key usage: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Usage for %s:\n", name)
	fmt.Printf("Last Used: %s\n", usage["last_used"])
	fmt.Printf("Request Count (24h): %d\n", usage["request_count"])
	fmt.Printf("Failed Attempts: %d\n", usage["failed_attempts"])
	fmt.Printf("Created: %s\n", usage["created_at"])
	fmt.Printf("Expires: %s\n", usage["expires_at"])
}

// maskToken masks a token for display
func maskToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
