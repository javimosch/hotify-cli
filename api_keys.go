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
	keys       map[string]*APIKey  // name -> key
	keysByToken map[string]*APIKey  // token -> key
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
		keys:       make(map[string]*APIKey),
		keysByToken: make(map[string]*APIKey),
		permMgr:    NewPermissionManager(),
		audit:      audit,
		security:   security,
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
	a.keysByToken[key.Token] = key
	a.permMgr.apiKeys = a.keysByToken

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
	delete(a.keysByToken, key.Token)
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

	// Update maps
	delete(a.keysByToken, oldToken)
	a.keysByToken[newToken] = key
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

// keyToMap converts an API key to a map for JSON output
func keyToMap(key *APIKey) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":            key.Name,
		"permissions":     key.Permissions,
		"created_at":      key.CreatedAt.Format(time.RFC3339),
		"last_used":       key.LastUsed.Format(time.RFC3339),
		"request_count":   key.RequestCount,
		"failed_attempts": key.FailedAttempts,
		"expires_at":      key.ExpiresAt.Format(time.RFC3339),
	}, nil
}

// handleAPIKeysCLI handles the api-keys CLI command
func handleAPIKeysCLI() {
	apiKeysCmd := flag.NewFlagSet("api-keys", flag.ExitOnError)
	action := apiKeysCmd.String("action", "list", "Action: add, list, remove, regenerate, permissions, usage")
	name := apiKeysCmd.String("name", "", "API key name")
	token := apiKeysCmd.String("token", "auto", "API token (auto to generate)")
	permissions := apiKeysCmd.String("permissions", "", "Comma-separated permissions")
	addPerms := apiKeysCmd.String("add", "", "Permissions to add")
	removePerms := apiKeysCmd.String("remove", "", "Permissions to remove")
	
	// Filter out --human flag before parsing
	filteredArgs := filterHumanFlag(os.Args[2:])
	apiKeysCmd.Parse(filteredArgs)

	// Determine output format (JSON by default, --human for text)
	format := getOutputFormat()

	switch *action {
	case "add":
		handleAPIKeyAdd(*name, *token, *permissions, format)
	case "list":
		handleAPIKeyList(format)
	case "remove":
		handleAPIKeyRemove(*name, format)
	case "regenerate":
		handleAPIKeyRegenerate(*name, format)
	case "permissions":
		handleAPIKeyPermissions(*name, *addPerms, *removePerms, format)
	case "usage":
		handleAPIKeyUsage(*name, format)
	default:
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "invalid_argument",
				Message:     fmt.Sprintf("Unknown action: %s", *action),
				Recoverable: false,
				Suggestions: []string{"Valid actions: add, list, remove, regenerate, permissions, usage"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}
}

func handleAPIKeyAdd(name, token, permissions string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli api-keys --action add --name <name> [--token <token>] [--permissions <permissions>]"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	// Parse permissions
	var perms []Permission
	var err error
	if permissions != "" {
		perms, err = ParsePermissions(permissions)
		if err != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitInvalidArgument,
					Type:        "invalid_permissions",
					Message:     fmt.Sprintf("Error parsing permissions: %v", err),
					Recoverable: false,
					Suggestions: []string{"Valid permissions: deploy, start, stop, logs, config, admin"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
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
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	key, err := manager.AddKey(name, perms, tokenValue)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "key_creation_failed",
				Message:     fmt.Sprintf("Error adding API key: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check if key name already exists", "Verify permissions are valid"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	keyData, _ := keyToMap(key)
	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"key": keyData,
		},
		Metadata: map[string]interface{}{
			"action": "created",
			"warning": "Store this token securely. It will not be shown again.",
		},
	}
	printOutput(result, format)
}

func handleAPIKeyList(format OutputFormat) {
	manager, err := NewAPIKeyManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	keys := manager.ListKeys()
	
	keysData := []map[string]interface{}{}
	for _, key := range keys {
		keyData, _ := keyToMap(key)
		keysData = append(keysData, keyData)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"keys":  keysData,
			"count": len(keysData),
		},
	}
	printOutput(result, format)
}

func handleAPIKeyRemove(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli api-keys --action remove --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	if err := manager.RemoveKey(name); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "key_not_found",
				Message:     fmt.Sprintf("Error removing API key: %v", err),
				Recoverable: false,
				Suggestions: []string{"List available keys: hotify-cli api-keys --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"removed_key": name,
		},
		Metadata: map[string]interface{}{
			"action": "removed",
		},
	}
	printOutput(result, format)
}

func handleAPIKeyRegenerate(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli api-keys --action regenerate --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	key, err := manager.RegenerateKey(name)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "key_not_found",
				Message:     fmt.Sprintf("Error regenerating API key: %v", err),
				Recoverable: false,
				Suggestions: []string{"List available keys: hotify-cli api-keys --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	keyData, _ := keyToMap(key)
	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"key": keyData,
		},
		Metadata: map[string]interface{}{
			"action": "regenerated",
			"warning": "Update your authentication with this new token.",
		},
	}
	printOutput(result, format)
}

func handleAPIKeyPermissions(name, add, remove string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli api-keys --action permissions --name <name> [--add <perms>] [--remove <perms>]"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	var addPerms, removePerms []Permission
	var err1, err2 error

	if add != "" {
		addPerms, err1 = ParsePermissions(add)
		if err1 != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitInvalidArgument,
					Type:        "invalid_permissions",
					Message:     fmt.Sprintf("Error parsing add permissions: %v", err1),
					Recoverable: false,
					Suggestions: []string{"Valid permissions: deploy, start, stop, logs, config, admin"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
		}
	}

	if remove != "" {
		removePerms, err2 = ParsePermissions(remove)
		if err2 != nil {
			result := CommandResult{
				Version: "1.0",
				Success: false,
				Error: &CommandError{
					Code:        ExitInvalidArgument,
					Type:        "invalid_permissions",
					Message:     fmt.Sprintf("Error parsing remove permissions: %v", err2),
					Recoverable: false,
					Suggestions: []string{"Valid permissions: deploy, start, stop, logs, config, admin"},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
		}
	}

	if err := manager.UpdatePermissions(name, addPerms, removePerms); err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "permission_update_failed",
				Message:     fmt.Sprintf("Error updating permissions: %v", err),
				Recoverable: false,
				Suggestions: []string{"Check if key exists", "Verify permissions are valid"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"key": name,
			"added_permissions":   addPerms,
			"removed_permissions": removePerms,
		},
		Metadata: map[string]interface{}{
			"action": "permissions_updated",
		},
	}
	printOutput(result, format)
}

func handleAPIKeyUsage(name string, format OutputFormat) {
	if name == "" {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitInvalidArgument,
				Type:        "missing_required_flags",
				Message:     "Missing required flag: --name",
				Recoverable: false,
				Suggestions: []string{"Usage: hotify-cli api-keys --action usage --name <name>"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitInvalidArgument)
	}

	manager, err := NewAPIKeyManager()
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitGenericFailure,
				Type:        "manager_creation_failed",
				Message:     fmt.Sprintf("Error creating API key manager: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitGenericFailure)
	}

	usage, err := manager.GetKeyUsage(name)
	if err != nil {
		result := CommandResult{
			Version: "1.0",
			Success: false,
			Error: &CommandError{
				Code:        ExitTargetNotFound,
				Type:        "key_not_found",
				Message:     fmt.Sprintf("Error getting key usage: %v", err),
				Recoverable: false,
				Suggestions: []string{"List available keys: hotify-cli api-keys --action list"},
			},
		}
		printOutput(result, format)
		os.Exit(ExitTargetNotFound)
	}

	result := CommandResult{
		Version: "1.0",
		Success: true,
		Data: map[string]interface{}{
			"key":   name,
			"usage": usage,
		},
	}
	printOutput(result, format)
}

// maskToken masks a token for display
func maskToken(token string) string {
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
