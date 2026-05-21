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

// CheckTokenPermission checks if a token has a specific permission (using global apiKeyManager)
func CheckTokenPermission(token string, permission Permission) bool {
	if apiKeyManager == nil {
		return false
	}

	key, err := apiKeyManager.GetKeyByToken(token)
	if err != nil {
		return false
	}

	// Admin has all permissions
	for _, perm := range key.Permissions {
		if perm == PermissionAdmin {
			return true
		}
	}

	// "all" or "*" grants all permissions
	for _, perm := range key.Permissions {
		if string(perm) == "all" || string(perm) == "*" {
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

// CheckTokenPermissions checks if a token has all of the specified permissions
func CheckTokenPermissions(token string, permissions []Permission) bool {
	for _, perm := range permissions {
		if !CheckTokenPermission(token, perm) {
			return false
		}
	}
	return true
}

// CheckTokenAnyPermission checks if a token has any of the specified permissions
func CheckTokenAnyPermission(token string, permissions []Permission) bool {
	for _, perm := range permissions {
		if CheckTokenPermission(token, perm) {
			return true
		}
	}
	return false
}

// ValidatePermission checks if a permission string is valid
func ValidatePermission(perm string) bool {
	// Accept wildcards for full access
	if perm == "all" || perm == "*" {
		return true
	}
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

		// Expand wildcards to all permissions
		if perm == "all" || perm == "*" {
			return AllPermissions, nil
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

// EndpointPermission defines required permissions for API endpoints
type EndpointPermission struct {
	Path        string
	Method      string
	Permissions []Permission
	Description string
}

// EndpointPermissions maps API endpoints to their required permissions
var EndpointPermissions = []EndpointPermission{
	// Status & Config
	{"/api/status", "GET", []Permission{PermissionLogs}, "Get daemon status"},
	{"/api/config", "GET", []Permission{PermissionConfig}, "Get configuration"},
	{"/api/config", "POST", []Permission{PermissionConfig}, "Update configuration"},

	// App Management
	{"/api/apps", "GET", []Permission{PermissionConfig}, "List all apps"},
	{"/api/apps/add", "POST", []Permission{PermissionConfig}, "Add new app"},
	{"/api/apps/edit", "POST", []Permission{PermissionConfig}, "Edit app"},
	{"/api/apps/remove", "POST", []Permission{PermissionConfig}, "Remove app"},

	// App Operations
	{"/api/apps/*/start", "POST", []Permission{PermissionStart}, "Start app"},
	{"/api/apps/*/stop", "POST", []Permission{PermissionStop}, "Stop app"},
	{"/api/apps/*/restart", "POST", []Permission{PermissionRestart}, "Restart app"},
	{"/api/apps/*/pause", "POST", []Permission{PermissionStop}, "Pause app"},
	{"/api/apps/*/resume", "POST", []Permission{PermissionStart}, "Resume app"},
	{"/api/apps/*/status", "GET", []Permission{PermissionLogs}, "Get app status"},
	{"/api/apps/*/logs", "GET", []Permission{PermissionLogs}, "Get app logs"},

	// Deployment
	{"/api/deploy", "POST", []Permission{PermissionDeploy}, "Deploy app"},

	// Docker Compose
	{"/api/compose/deploy", "POST", []Permission{PermissionDeploy}, "Deploy compose"},
	{"/api/compose/sync", "POST", []Permission{PermissionDeploy}, "Sync compose"},
	{"/api/compose/copy-dir", "POST", []Permission{PermissionDeploy}, "Copy directory"},
	{"/api/compose/volume-init", "POST", []Permission{PermissionDeploy}, "Initialize volume"},

	// Remote App Operations (v2.7.3+)
	{"/api/remote/apps/*/basic-auth", "POST", []Permission{PermissionConfig}, "Manage basic auth"},
	{"/api/remote/apps/*/setup-traefik", "POST", []Permission{PermissionConfig}, "Setup Traefik"},
	{"/api/remote/apps/*/setup-dns", "POST", []Permission{PermissionConfig}, "Setup DNS"},

	// Traefik System
	{"/api/traefik-system/status", "GET", []Permission{PermissionConfig}, "Get Traefik status"},
	{"/api/traefik-system/install", "POST", []Permission{PermissionConfig}, "Install Traefik"},
	{"/api/traefik-system/remove", "POST", []Permission{PermissionConfig}, "Remove Traefik"},

	// API Keys (requires admin)
	{"/api/api-keys", "GET", []Permission{PermissionAdmin}, "List API keys"},
	{"/api/api-keys", "POST", []Permission{PermissionAdmin}, "Create API key"},
	{"/api/api-keys/*", "GET", []Permission{PermissionAdmin}, "Get API key details"},
	{"/api/api-keys/*", "DELETE", []Permission{PermissionAdmin}, "Delete API key"},
	{"/api/api-keys/*", "POST", []Permission{PermissionAdmin}, "Update API key"},
}

// GetRequiredPermissions returns required permissions for an endpoint
func GetRequiredPermissions(path, method string) []Permission {
	// Check exact matches first
	for _, ep := range EndpointPermissions {
		if ep.Path == path && ep.Method == method {
			return ep.Permissions
		}
	}

	// Check wildcard matches
	for _, ep := range EndpointPermissions {
		if matchWildcardPath(ep.Path, path) && ep.Method == method {
			return ep.Permissions
		}
	}

	// Default: require admin permission for unknown endpoints
	return []Permission{PermissionAdmin}
}

// matchWildcardPath checks if a pattern matches a path (supports * wildcard)
func matchWildcardPath(pattern, path string) bool {
	if pattern == path {
		return true
	}

	// Simple wildcard matching
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, part := range patternParts {
		if part != "*" && part != pathParts[i] {
			return false
		}
	}

	return true
}
