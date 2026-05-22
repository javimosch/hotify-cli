package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDir  = ".hotify"
	configFile = "config.json"
)

// OutputFormat controls command output format
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

// getOutputFormat determines the output format based on command-line flags
func getOutputFormat() OutputFormat {
	args := os.Args[2:]
	for _, arg := range args {
		if arg == "--human" || strings.HasPrefix(arg, "--human=") {
			return OutputFormatText
		}
	}
	return OutputFormatJSON // JSON by default
}

// filterHumanFlag removes --human flag from args to avoid flag parsing conflicts
func filterHumanFlag(args []string) []string {
	filtered := []string{}
	for _, arg := range args {
		if arg != "--human" && !strings.HasPrefix(arg, "--human=") {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

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
	ExitInvalidArgument       = 97
	ExitConfigError           = 98
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

type Config struct {
	CloudflareToken string        `json:"cloudflare_token"`
	Domain          string        `json:"domain"`
	AdminEmail      string        `json:"admin_email"`
	Apps            []App         `json:"apps"`
	Security        SecurityConfig `json:"security"`
	Remotes         []Remote      `json:"remotes"`
}

type Remote struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	SSHHost      string   `json:"ssh_host,omitempty"`
	AuthToken    string   `json:"auth_token"`
	Permissions  []string `json:"permissions"`
	Default      bool     `json:"default"`
	LastUsed     string   `json:"last_used,omitempty"`
}

type SecurityConfig struct {
	EncryptionKey           string `json:"encryption_key"`
	TokenExpirationDays     int    `json:"token_expiration_days"`
	MaxFailedAttempts       int    `json:"max_failed_attempts"`
	RateLimitPerMinute      int    `json:"rate_limit_per_minute"`
	RequireHTTPS            bool   `json:"require_https"`
	AllowedIPs              []string `json:"allowed_ips"`
	AuditLogRetentionDays   int    `json:"audit_log_retention_days"`
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		TokenExpirationDays:   30,
		MaxFailedAttempts:     5,
		RateLimitPerMinute:    60,
		RequireHTTPS:         true,
		AllowedIPs:           []string{},
		AuditLogRetentionDays: 90,
	}
}

type App struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	Port       int    `json:"port"`
	Command    string `json:"command"`
	Source     string `json:"source,omitempty"`
	Status     string `json:"status"`
	// Deployment fields
	RemotePath string `json:"remote_path,omitempty"`
	BuildType  string `json:"build_type,omitempty"`
	Remote     string `json:"remote,omitempty"`
	PID        int    `json:"pid,omitempty"`
	// Docker Compose fields
	ComposeFile string `json:"compose_file,omitempty"`
	ComposePath string `json:"compose_path,omitempty"`
	// Traefik basic auth — htpasswd-format entries ("user:$apr1$...")
	// Populated via: hotify-cli basic-auth --id <app> --action add --user <u> --password <p>
	BasicAuth []string `json:"basic_auth,omitempty"`
	// Backend URL for external reverse proxy targets
	// If set, Traefik will route to this URL instead of http://127.0.0.1:<port>
	// Example: "http://100.114.4.57:8080" for Tailscale network
	BackendURL string `json:"backend_url,omitempty"`
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting home directory: %v", err)
	}
	configPath := filepath.Join(homeDir, configDir, configFile)
	return configPath, nil
}

func loadConfig() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	// Create config directory if it doesn't exist
	configDirPath := filepath.Dir(configPath)
	if _, err := os.Stat(configDirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(configDirPath, 0755); err != nil {
			return nil, fmt.Errorf("error creating config directory: %v", err)
		}
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			Apps:     []App{},
			Security: DefaultSecurityConfig(),
			Remotes:  []Remote{},
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	return &config, nil
}

func saveConfig(config *Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %v", err)
	}

	return nil
}

func initConfig() {
	format := getOutputFormat()

	// Parse flags for non-interactive mode
	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	token := initCmd.String("token", "", "Cloudflare API token")
	domain := initCmd.String("domain", "", "Base domain (e.g., example.com)")
	email := initCmd.String("email", "", "Admin email for Let's Encrypt")
	filteredArgs := filterHumanFlag(os.Args[2:])
	initCmd.Parse(filteredArgs)

	config, err := loadConfig()
	if err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitConfigError,
				Type:        "config_error",
				Message:     fmt.Sprintf("Error loading config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	if format == OutputFormatJSON {
		// Non-interactive: require flags
		if *token == "" || *domain == "" || *email == "" {
			result := CommandResult{
				Version: Version,
				Success: false,
				Error: &CommandError{
					Code:        ExitInvalidArgument,
					Type:        "validation_error",
					Message:     "Non-interactive mode requires --token, --domain, and --email flags",
					Recoverable: false,
					Suggestions: []string{
						"hotify-cli init --token <cf-token> --domain <domain> --email <email>",
						"Use --human flag for interactive prompts",
					},
				},
			}
			printOutput(result, format)
			os.Exit(ExitInvalidArgument)
		}
		config.CloudflareToken = *token
		config.Domain = *domain
		config.AdminEmail = *email
	} else {
		// Interactive (--human) mode: use flags if provided, else prompt
		cfToken := *token
		if cfToken == "" {
			fmt.Print("Enter Cloudflare API token: ")
			fmt.Scanln(&cfToken)
		}
		baseDomain := *domain
		if baseDomain == "" {
			fmt.Print("Enter base domain (e.g., example.com): ")
			fmt.Scanln(&baseDomain)
		}
		adminEmail := *email
		if adminEmail == "" {
			fmt.Print("Enter admin email for Let's Encrypt: ")
			fmt.Scanln(&adminEmail)
		}
		config.CloudflareToken = cfToken
		config.Domain = baseDomain
		config.AdminEmail = adminEmail
	}

	if err := saveConfig(config); err != nil {
		result := CommandResult{
			Version: Version,
			Success: false,
			Error: &CommandError{
				Code:        ExitConfigError,
				Type:        "config_error",
				Message:     fmt.Sprintf("Error saving config: %v", err),
				Recoverable: false,
			},
		}
		printOutput(result, format)
		os.Exit(ExitConfigError)
	}

	configPath, _ := getConfigPath()

	// Warn if Traefik is not installed
	warnings := []string{}
	if _, err := checkTraefikInstalled(); err != nil {
		warnings = append(warnings, "Traefik is not installed. Run: hotify-cli traefik-system to install it")
	}

	result := CommandResult{
		Version: Version,
		Success: true,
		Data: map[string]interface{}{
			"config_path": configPath,
			"domain":      config.Domain,
			"email":       config.AdminEmail,
		},
		Metadata: map[string]interface{}{
			"warnings": warnings,
		},
	}
	printOutput(result, format)
}

// app management (setup/add/edit/remove/list) → see apps.go
