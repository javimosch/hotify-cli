package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = ".hotify"
	configFile = "config.json"
)

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
	fmt.Println("Initializing hotify-cli configuration...")

	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Prompt for Cloudflare token
	fmt.Print("Enter Cloudflare API token: ")
	var cfToken string
	fmt.Scanln(&cfToken)
	config.CloudflareToken = cfToken

	// Prompt for domain
	fmt.Print("Enter base domain (e.g., example.com): ")
	var domain string
	fmt.Scanln(&domain)
	config.Domain = domain

	// Prompt for admin email
	fmt.Print("Enter admin email for Let's Encrypt: ")
	var email string
	fmt.Scanln(&email)
	config.AdminEmail = email

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	configPath, _ := getConfigPath()
	fmt.Println("Configuration saved successfully!")
	fmt.Printf("Config file: %s\n", configPath)
}

func addApp() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Skip Cloudflare check for deployment-only use
	// if config.CloudflareToken == "" || config.Domain == "" || config.AdminEmail == "" {
	// 	fmt.Println("Please run 'hotify-cli init' first to set up configuration")
	// 	os.Exit(1)
	// }

	// Parse flags
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	id := addCmd.String("id", "", "App ID (required)")
	name := addCmd.String("name", "", "App name (required)")
	domain := addCmd.String("domain", "", "App subdomain (required)")
	port := addCmd.Int("port", 0, "App port (required)")
	command := addCmd.String("command", "", "Command to start app (required)")
	source := addCmd.String("source", "", "App source (optional)")
	addCmd.Parse(os.Args[2:])

	if *id == "" || *name == "" || *domain == "" || *port == 0 || *command == "" {
		fmt.Println("Missing required flags")
		fmt.Println("Usage: hotify-cli add --id <id> --name <name> --domain <domain> --port <port> --command <command> [--source <source>]")
		os.Exit(1)
	}

	// Check if app ID already exists
	for _, app := range config.Apps {
		if app.ID == *id {
			fmt.Printf("App with ID '%s' already exists\n", *id)
			os.Exit(1)
		}
	}

	// Create full domain
	fullDomain := fmt.Sprintf("%s.%s", *domain, config.Domain)

	// Add app to config
	newApp := App{
		ID:      *id,
		Name:    *name,
		Domain:  fullDomain,
		Port:    *port,
		Command: *command,
		Source:  *source,
		Status:  "stopped",
	}

	config.Apps = append(config.Apps, newApp)

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Added app: %s (%s)\n", *name, fullDomain)
	fmt.Println("Next steps:")
	fmt.Printf("1. Deploy the app to the server\n")
	fmt.Printf("2. Run DNS setup: hotify-cli setup-dns --id %s\n", *id)
	fmt.Printf("3. Run Traefik setup: hotify-cli setup-traefik --id %s\n", *id)
}

func editApp() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse flags
	editCmd := flag.NewFlagSet("edit", flag.ExitOnError)
	id := editCmd.String("id", "", "App ID (required)")
	name := editCmd.String("name", "", "App name")
	domain := editCmd.String("domain", "", "App subdomain")
	port := editCmd.Int("port", 0, "App port")
	command := editCmd.String("command", "", "Command to start app")
	source := editCmd.String("source", "", "App source")
	editCmd.Parse(os.Args[2:])

	if *id == "" {
		fmt.Println("Missing required flag: --id")
		fmt.Println("Usage: hotify-cli edit --id <id> [--name <name>] [--domain <domain>] [--port <port>] [--command <command>] [--source <source>]")
		os.Exit(1)
	}

	// Find and update app
	found := false
	for i, app := range config.Apps {
		if app.ID == *id {
			found = true
			if *name != "" {
				config.Apps[i].Name = *name
			}
			if *domain != "" {
				config.Apps[i].Domain = fmt.Sprintf("%s.%s", *domain, config.Domain)
			}
			if *port != 0 {
				config.Apps[i].Port = *port
			}
			if *command != "" {
				config.Apps[i].Command = *command
			}
			if *source != "" {
				config.Apps[i].Source = *source
			}
			break
		}
	}

	if !found {
		fmt.Printf("App with ID '%s' not found\n", *id)
		os.Exit(1)
	}

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Updated app: %s\n", *id)
}

func removeApp() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Parse flags
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
	id := removeCmd.String("id", "", "App ID (required)")
	removeCmd.Parse(os.Args[2:])

	if *id == "" {
		fmt.Println("Missing required flag: --id")
		fmt.Println("Usage: hotify-cli remove --id <id>")
		os.Exit(1)
	}

	// Find and remove app
	found := false
	var updatedApps []App
	for _, app := range config.Apps {
		if app.ID != *id {
			updatedApps = append(updatedApps, app)
		} else {
			found = true
		}
	}

	if !found {
		fmt.Printf("App with ID '%s' not found\n", *id)
		os.Exit(1)
	}

	config.Apps = updatedApps

	if err := saveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Removed app: %s\n", *id)
}

func listApps() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Apps) == 0 {
		fmt.Println("No apps configured")
		return
	}

	fmt.Println("Configured Apps:")
	fmt.Println("================")
	for _, app := range config.Apps {
		fmt.Printf("ID: %s\n", app.ID)
		fmt.Printf("  Name: %s\n", app.Name)
		fmt.Printf("  Domain: %s\n", app.Domain)
		fmt.Printf("  Port: %d\n", app.Port)
		fmt.Printf("  Command: %s\n", app.Command)
		if app.Source != "" {
			fmt.Printf("  Source: %s\n", app.Source)
		}
		fmt.Printf("  Status: %s\n", app.Status)
		fmt.Println()
	}
}
