package main

import (
	"fmt"
	"strings"
)

// Permission represents a permission level
type Permission string

const (
	PermissionDeploy  Permission = "deploy"
	PermissionStart   Permission = "start"
	PermissionStop    Permission = "stop"
	PermissionRestart Permission = "restart"
	PermissionLogs    Permission = "logs"
	PermissionConfig  Permission = "config"
	PermissionAdmin   Permission = "admin"
)

// AllPermissions lists all available permissions
var AllPermissions = []Permission{
	PermissionDeploy,
	PermissionStart,
	PermissionStop,
	PermissionRestart,
	PermissionLogs,
	PermissionConfig,
	PermissionAdmin,
}

// PermissionManager manages permissions for API keys
type PermissionManager struct {
	apiKeys map[string]*APIKey
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		apiKeys: make(map[string]*APIKey),
	}
}

// CheckPermission checks if a token has a specific permission
func (p *PermissionManager) CheckPermission(token string, permission Permission) bool {
	key, exists := p.apiKeys[token]
	if !exists {
		return false
	}

	// Admin has all permissions
	for _, perm := range key.Permissions {
		if perm == PermissionAdmin {
			return true
		}
	}

	// Check specific permission
	for _, perm := range key.Permissions {
		if perm == permission {
			return true
		}
	}

	return false
}

// GrantPermission grants a permission to an API key
func (p *PermissionManager) GrantPermission(token string, permission Permission) error {
	key, exists := p.apiKeys[token]
	if !exists {
		return fmt.Errorf("API key not found")
	}

	// Check if already has permission
	for _, perm := range key.Permissions {
		if perm == permission {
			return nil // Already has permission
		}
	}

	key.Permissions = append(key.Permissions, permission)
	return nil
}

// RevokePermission revokes a permission from an API key
func (p *PermissionManager) RevokePermission(token string, permission Permission) error {
	key, exists := p.apiKeys[token]
	if !exists {
		return fmt.Errorf("API key not found")
	}

	// Cannot revoke admin permission from admin key
	if permission == PermissionAdmin {
		return fmt.Errorf("cannot revoke admin permission")
	}

	// Remove permission
	var updatedPermissions []Permission
	for _, perm := range key.Permissions {
		if perm != permission {
			updatedPermissions = append(updatedPermissions, perm)
		}
	}

	key.Permissions = updatedPermissions
	return nil
}

// GetPermissions returns all permissions for a token
func (p *PermissionManager) GetPermissions(token string) ([]Permission, error) {
	key, exists := p.apiKeys[token]
	if !exists {
		return nil, fmt.Errorf("API key not found")
	}
	return key.Permissions, nil
}

// ValidatePermission checks if a permission string is valid
func ValidatePermission(perm string) bool {
	for _, p := range AllPermissions {
		if string(p) == perm {
			return true
		}
	}
	return false
}

// ParsePermissions parses a comma-separated list of permissions
func ParsePermissions(permString string) ([]Permission, error) {
	if permString == "" {
		return []Permission{}, nil
	}

	parts := strings.Split(permString, ",")
	var permissions []Permission

	for _, part := range parts {
		perm := strings.TrimSpace(part)
		if !ValidatePermission(perm) {
			return nil, fmt.Errorf("invalid permission: %s", perm)
		}
		permissions = append(permissions, Permission(perm))
	}

	return permissions, nil
}

// PermissionsToString converts permissions to a comma-separated string
func PermissionsToString(permissions []Permission) string {
	if len(permissions) == 0 {
		return ""
	}

	permStrings := make([]string, len(permissions))
	for i, perm := range permissions {
		permStrings[i] = string(perm)
	}
	return strings.Join(permStrings, ",")
}

// HasAnyPermission checks if token has any of the specified permissions
func (p *PermissionManager) HasAnyPermission(token string, permissions []Permission) bool {
	for _, perm := range permissions {
		if p.CheckPermission(token, perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if token has all of the specified permissions
func (p *PermissionManager) HasAllPermissions(token string, permissions []Permission) bool {
	for _, perm := range permissions {
		if !p.CheckPermission(token, perm) {
			return false
		}
	}
	return true
}
